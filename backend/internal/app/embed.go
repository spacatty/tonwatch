package app

import (
	"context"

	"fragwatch/internal/jobs"
	"fragwatch/internal/llm"
	"fragwatch/internal/vector"
)

const embedBatch = 256

// runEmbed embeds every name worth suggesting that has no vector yet (or whose text changed),
// then drops the in-memory index so the next search loads the new vectors.
func (a *App) runEmbed(ctx context.Context, j *jobs.Job) error {
	if !a.LLM.Enabled() {
		j.Progress("OPENROUTER_API_KEY not set — skipping")
		return nil
	}
	todo, _, err := a.Store.CountToEmbed(ctx, a.Cfg.EmbedModel)
	if err != nil {
		return err
	}
	_, err = a.EmbedPending(ctx, -1, func(done int) {
		j.Progress("embedded %d/%d names · session cost $%.4f", done, todo, a.LLMUsage().Cost)
	})
	return err
}

// EmbedPending embeds up to limit names (-1 = all) that need a vector and returns how many it did.
func (a *App) EmbedPending(ctx context.Context, limit int, progress func(done int)) (int, error) {
	done := 0
	defer a.dropVectors()
	for ctx.Err() == nil && (limit < 0 || done < limit) {
		n := embedBatch
		if limit >= 0 {
			n = min(n, limit-done)
		}
		rows, err := a.Store.ToEmbed(ctx, a.Cfg.EmbedModel, n)
		if err != nil || len(rows) == 0 {
			return done, err
		}
		texts := make([]string, len(rows))
		for i, r := range rows {
			texts[i] = r.Src
		}
		vecs, usage, err := a.LLM.Embed(ctx, a.Cfg.EmbedModel, a.Cfg.EmbedDims, texts)
		a.addUsage(usage)
		if err != nil {
			return done, err
		}
		for i, v := range vecs {
			vector.Normalize(v)
			rows[i].Vec, rows[i].Scale = vector.Quantize(v)
		}
		if err := a.Store.SaveVectors(ctx, a.Cfg.EmbedModel, rows); err != nil {
			return done, err
		}
		done += len(rows)
		if progress != nil {
			progress(done)
		}
	}
	return done, ctx.Err()
}

func (a *App) dropVectors() {
	a.vecMu.Lock()
	a.vecs = nil
	a.vecMu.Unlock()
}

// vectors returns the semantic index, loading it from the database on first use.
func (a *App) vectors(ctx context.Context) (*vector.Index, error) {
	a.vecMu.Lock()
	defer a.vecMu.Unlock()
	if a.vecs != nil {
		return a.vecs, nil
	}
	x := vector.New(a.Cfg.EmbedDims)
	if err := a.Store.EachVector(ctx, a.Cfg.EmbedModel, a.Cfg.EmbedDims, x.Add); err != nil {
		return nil, err
	}
	a.vecs = x
	return x, nil
}

// semantic returns up to k names whose meaning is closest to the text. No index yet means no
// names, not an error: Ask AI then works from keywords alone.
func (a *App) semantic(ctx context.Context, text string, k int) ([]string, llm.Usage, error) {
	var usage llm.Usage
	x, err := a.vectors(ctx)
	if err != nil || x.Len() == 0 {
		return nil, usage, err
	}
	vecs, usage, err := a.LLM.Embed(ctx, a.Cfg.EmbedModel, a.Cfg.EmbedDims, []string{text})
	if err != nil {
		return nil, usage, err
	}
	vector.Normalize(vecs[0])
	hits := x.Search(vecs[0], k)
	names := make([]string, len(hits))
	for i, h := range hits {
		names[i] = h.Name
	}
	return names, usage, nil
}
