package vector

import (
	"math"
	"testing"
)

func TestSearchFindsTheClosest(t *testing.T) {
	x := New(3)
	for name, v := range map[string][]float32{
		"coffee": {1, 0, 0},
		"tea":    {0.9, 0.1, 0},
		"garden": {0, 1, 0},
		"river":  {0, 0, 1},
	} {
		Normalize(v)
		q, s := Quantize(v)
		x.Add(name, q, s)
	}
	x.Add("wrong size", []int8{1}, 1)
	if x.Len() != 4 {
		t.Fatalf("len %d", x.Len())
	}
	q := []float32{1, 0.05, 0}
	Normalize(q)
	hits := x.Search(q, 2)
	if len(hits) != 2 || hits[0].Name != "coffee" || hits[1].Name != "tea" {
		t.Fatalf("hits %+v", hits)
	}
	if math.Abs(float64(hits[0].Score)-0.9988) > 0.01 {
		t.Errorf("cosine after quantising: %v", hits[0].Score)
	}
}

func TestQuantizeRoundTrip(t *testing.T) {
	v := []float32{0.5, -0.25, 0.125, 0}
	q, s := Quantize(v)
	for i := range v {
		if d := math.Abs(float64(float32(q[i])*s - v[i])); d > 0.004 {
			t.Errorf("%d: %v vs %v", i, float32(q[i])*s, v[i])
		}
	}
}
