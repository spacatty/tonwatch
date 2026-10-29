// Package vector keeps name embeddings in memory for search by meaning. Each vector is stored as
// int8 with one float scale (a quarter of the float32 size): 128k names of 256 dimensions take
// about 33 MB, and a brute-force scan of all of them takes a few tens of milliseconds.
package vector

import (
	"math"
	"sort"
)

// Quantize turns a vector into int8 values and the scale that maps them back (v ≈ q * scale).
func Quantize(v []float32) ([]int8, float32) {
	var hi float32
	for _, x := range v {
		hi = max(hi, float32(math.Abs(float64(x))))
	}
	q := make([]int8, len(v))
	if hi == 0 {
		return q, 0
	}
	scale := hi / 127
	for i, x := range v {
		q[i] = int8(max(-127, min(127, math.Round(float64(x/scale)))))
	}
	return q, scale
}

// Normalize scales v to unit length in place, so a dot product is the cosine similarity.
func Normalize(v []float32) {
	var s float64
	for _, x := range v {
		s += float64(x) * float64(x)
	}
	if s == 0 {
		return
	}
	n := float32(1 / math.Sqrt(s))
	for i := range v {
		v[i] *= n
	}
}

// Index is a flat, read-only set of quantised vectors. Build a new one to change it.
type Index struct {
	Dims   int
	names  []string
	vals   []int8 // len(names) * Dims
	scales []float32
}

func New(dims int) *Index { return &Index{Dims: dims} }

// Add appends one quantised vector; one of the wrong size is ignored.
func (x *Index) Add(name string, q []int8, scale float32) {
	if len(q) != x.Dims {
		return
	}
	x.names = append(x.names, name)
	x.vals = append(x.vals, q...)
	x.scales = append(x.scales, scale)
}

func (x *Index) Len() int {
	if x == nil {
		return 0
	}
	return len(x.names)
}

// Hit is one search result: a name and its cosine similarity to the query.
type Hit struct {
	Name  string
	Score float32
}

// Search returns the k names closest to the unit-length query, best first.
func (x *Index) Search(query []float32, k int) []Hit {
	if x.Len() == 0 || len(query) != x.Dims || k <= 0 {
		return nil
	}
	// a min-heap of the best k would be faster; a bounded slice kept sorted is plenty for k ≈ 100
	best := make([]Hit, 0, k+1)
	d := x.Dims
	for i, name := range x.names {
		row := x.vals[i*d : (i+1)*d]
		var dot float32
		for j, q := range row {
			dot += float32(q) * query[j]
		}
		s := dot * x.scales[i]
		if len(best) == k && s <= best[k-1].Score {
			continue
		}
		at := sort.Search(len(best), func(j int) bool { return best[j].Score < s })
		best = append(best, Hit{})
		copy(best[at+1:], best[at:])
		best[at] = Hit{name, s}
		if len(best) > k {
			best = best[:k]
		}
	}
	return best
}
