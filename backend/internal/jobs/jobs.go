// Package jobs runs the background collectors (one goroutine per job, never two of the same at once).
package jobs

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"
)

type Job struct {
	Name  string
	Desc  string
	Every time.Duration // 0 = manual only
	Run   func(ctx context.Context, j *Job) error

	mu       sync.Mutex
	running  bool
	cancel   context.CancelFunc
	progress string
	key      string   // the format progress was written with: the dashboard translates by it
	args     []string // its arguments, as text
	lastRun  time.Time
	lastDur  time.Duration
	lastErr  string
	nextRun  time.Time
}

// Progress updates the status line shown in the dashboard. The format doubles as a key: the
// dashboard looks it up to show the line in the reader's language, and falls back to this text.
func (j *Job) Progress(format string, a ...any) {
	args := make([]string, len(a))
	for i, v := range a {
		args[i] = fmt.Sprint(v)
	}
	j.mu.Lock()
	j.progress, j.key, j.args = fmt.Sprintf(format, a...), format, args
	j.mu.Unlock()
}

type Status struct {
	Name     string   `json:"name"`
	Desc     string   `json:"desc"`
	Running  bool     `json:"running"`
	Progress string   `json:"progress"`
	Key      string   `json:"progress_key,omitempty"`
	Args     []string `json:"progress_args,omitempty"`
	LastRun  int64    `json:"last_run"`
	LastDur  int64    `json:"last_duration_ms"`
	LastErr  string   `json:"last_error"`
	NextRun  int64    `json:"next_run"`
	EverySec int64    `json:"every_sec"`
}

type Manager struct {
	ctx  context.Context
	jobs map[string]*Job
}

func NewManager(ctx context.Context) *Manager { return &Manager{ctx: ctx, jobs: map[string]*Job{}} }

func (m *Manager) Add(j *Job) { m.jobs[j.Name] = j }

// Trigger starts a job now unless it's already running. Returns false if it was running.
func (m *Manager) Trigger(name string) (bool, error) {
	j, ok := m.jobs[name]
	if !ok {
		return false, fmt.Errorf("unknown job %q", name)
	}
	j.mu.Lock()
	if j.running {
		j.mu.Unlock()
		return false, nil
	}
	ctx, cancel := context.WithCancel(m.ctx)
	j.running, j.cancel, j.progress, j.key, j.args = true, cancel, "starting", "starting", nil
	j.mu.Unlock()

	go func() {
		start := time.Now()
		err := func() (err error) {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("panic: %v", r)
				}
			}()
			return j.Run(ctx, j)
		}()
		cancel()
		j.mu.Lock()
		j.running, j.lastRun, j.lastDur = false, start, time.Since(start)
		j.lastErr = ""
		if err != nil {
			j.lastErr = err.Error()
			log.Printf("job %s: %v", j.Name, err)
		}
		if j.Every > 0 {
			j.nextRun = time.Now().Add(j.Every)
		}
		j.mu.Unlock()
	}()
	return true, nil
}

func (m *Manager) Stop(name string) {
	if j, ok := m.jobs[name]; ok {
		j.mu.Lock()
		if j.cancel != nil {
			j.cancel()
		}
		j.mu.Unlock()
	}
}

// Schedule runs periodic jobs. initialDelay staggers the first runs.
func (m *Manager) Schedule(initialDelay map[string]time.Duration) {
	for _, j := range m.jobs {
		// only the jobs named here run on their own; the rest wait for a manual run
		if _, ok := initialDelay[j.Name]; ok && j.Every > 0 {
			j.mu.Lock()
			j.nextRun = time.Now().Add(initialDelay[j.Name])
			j.mu.Unlock()
		}
	}
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-m.ctx.Done():
				return
			case <-t.C:
			}
			for name, j := range m.jobs {
				j.mu.Lock()
				due := j.Every > 0 && !j.running && !j.nextRun.IsZero() && time.Now().After(j.nextRun)
				j.mu.Unlock()
				if due {
					m.Trigger(name)
				}
			}
		}
	}()
}

func (m *Manager) Statuses() []Status {
	var out []Status
	for _, j := range m.jobs {
		j.mu.Lock()
		s := Status{Name: j.Name, Desc: j.Desc, Running: j.running, Progress: j.progress, Key: j.key, Args: j.args, LastErr: j.lastErr,
			LastDur: j.lastDur.Milliseconds(), EverySec: int64(j.Every.Seconds())}
		if !j.lastRun.IsZero() {
			s.LastRun = j.lastRun.Unix()
		}
		if !j.nextRun.IsZero() {
			s.NextRun = j.nextRun.Unix()
		}
		j.mu.Unlock()
		out = append(out, s)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Name < out[b].Name })
	return out
}
