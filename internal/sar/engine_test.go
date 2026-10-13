package sar

import (
	"math"
	"testing"
)

func TestShiftWeightsLeftBranch(t *testing.T) {
	tuning := []int{0, 22, 44, 65, 87, 109, 131, 152, 174, 196, 218, 239, 261, 283, 305, 326, 348, 370, 392, 414, 435, 457, 479, 501, 522, 544, 566, 588, 609, 631, 653, 675, 697, 718, 740, 762, 784, 805, 827, 849, 871, 892, 914, 936, 958, 979, 1001, 1023}
	w := ShiftWeights(tuning, 24) // pivot = 522 -> right branch (mirrored)
	if math.Abs(w[24]-1.0) > 1e-9 {
		t.Fatalf("w[pivot] = %v, want 1", w[24])
	}
	if w[0] != 0 {
		t.Fatalf("w[0] = %v, want 0 (beyond mirror)", w[0])
	}
	for i := 1; i < 24; i++ {
		if w[i] < w[i-1] {
			t.Fatalf("not monotonic at %d: %v < %v", i, w[i], w[i-1])
		}
	}
	if w[47] >= w[30] {
		t.Fatalf("right side should decay: w[47]=%v w[30]=%v", w[47], w[30])
	}
}

func TestShiftWeightsPivotAtCenter(t *testing.T) {
	tuning := make([]int, 48)
	for i := range tuning {
		tuning[i] = i * 20
	}
	// pivot index 10 -> tuning 200 (< 512) -> left branch
	w := ShiftWeights(tuning, 10)
	if math.Abs(w[10]-1.0) > 1e-9 {
		t.Fatalf("w[pivot]=%v want 1", w[10])
	}
	if math.Abs(w[0]) > 1e-9 {
		t.Fatalf("w[0]=%v want 0", w[0])
	}
	// beyond 2*P (400) -> 0
	for i := 20; i < 48; i++ {
		if w[i] != 0 {
			t.Fatalf("w[%d]=%v want 0 beyond 2P", i, w[i])
		}
	}
}

func TestShiftModifiesCurveAtPoint(t *testing.T) {
	tab := GammaTable{Number: 11}
	for i := 0; i < 48; i++ {
		tab.Points = append(tab.Points, GammaPoint{QL: uint16(i * 20), Density: 0.5 + float64(i)*0.05})
	}
	p := DefaultParams()
	p.Points[0] = PointOfInterest{QL: 200, Density: 1.0, Shift: 50, Contrast: 100}
	p.Active = 1
	res := Compute(tab, p)
	orig := res.DensityWords[10]
	if res.ChangedWords[10] != orig+50 {
		t.Fatalf("pivot: %d != %d+50", res.ChangedWords[10], orig)
	}
	// far left unchanged
	if res.ChangedWords[0] != res.DensityWords[0] {
		t.Fatalf("far left changed: %d vs %d", res.ChangedWords[0], res.DensityWords[0])
	}
	// curve stays monotonic
	for i := 1; i < 48; i++ {
		if res.ChangedWords[i] < res.ChangedWords[i-1] {
			t.Fatalf("not monotonic at %d", i)
		}
	}
}
