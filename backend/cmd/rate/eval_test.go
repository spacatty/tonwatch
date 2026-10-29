package main

import (
	"math"
	"testing"
)

func TestSpearman(t *testing.T) {
	if r := spearman([]float64{1, 2, 3, 4}, []float64{10, 20, 30, 400}); math.Abs(r-1) > 1e-9 {
		t.Errorf("monotone: %v", r)
	}
	if r := spearman([]float64{1, 2, 3, 4}, []float64{4, 3, 2, 1}); math.Abs(r+1) > 1e-9 {
		t.Errorf("reversed: %v", r)
	}
	if got := ranks([]float64{5, 1, 5, 3}); got[0] != 3.5 || got[1] != 1 || got[2] != 3.5 || got[3] != 2 {
		t.Errorf("tied ranks: %v", got)
	}
	if !math.IsNaN(spearman([]float64{1, 1, 1}, []float64{1, 2, 3})) {
		t.Error("constant input has no correlation")
	}
}
