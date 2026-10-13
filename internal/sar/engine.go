// Package sar reimplements the computational core of "SAR MAKER for Win"
// (ocsarmk.exe, FujiPhotoFilm MiyaD1 2002-10-21), recovered by static
// reverse engineering of the original 32-bit MFC binary.
//
// Recovered primitives (original addresses in parentheses):
//
//   - Interp (sub_4018E0): linear interpolation
//     v = v1 + (x-k1)*(v2-v1)/(k2-k1)
//   - BuildLUT (sub_4017A0): builds the 1024-entry density LUT from the
//     48 tuning points and 48 density words. Each output word is
//     trunc(interp + 0.5).
//   - ShiftWeights (sub_40A680): smoothstep weight curve over the 48
//     tuning points, centred on one selected point (the "tuning point").
//     Left branch (P < 512): t = QL[i]/P, w = 3t^2-2t^3; right of the
//     pivot u = (2P-QL[i])/P, w = smoothstep(u), zero past 2P. Mirrored
//     branch for P >= 512 uses 1023-P and offsets by +1.0.
//   - Blend (sub_40A870): changed[i] = trunc(orig[i] + shift*w[i] + 0.5)
//     i.e. orig*w + (orig+shift)*(1-w).
//   - Contrast (sub_40A5D0): v' = trunc(v*c - (c-1)*anchor) with
//     c = contrast/100 and anchor = the point-of-interest density word.
//   - Clamp (sub_40A940): clamp to [DMIN,DMAX] and enforce a monotonic
//     non-decreasing curve (backward then forward pass).
//   - Density words are stored at 1/10000 density units:
//     word = trunc(density * 10000.0)  (fmul [0x40D110]=10000, ftol).
//   - DMAX/DMIN are stored at 1/100 density units (DMAX=300 == 3.00).
package sar

import "math"

const (
	// TuningCount is the number of tuning points (48 in every gamma table).
	TuningCount = 48
	// LutSize is the width of the built density LUT (1024 levels).
	LutSize = 1024
	// DensityScale is the fixed-point scale of density words (1e-4 units).
	DensityScale = 10000.0
	// POICount is the number of points of interest of the original UI.
	POICount = 5
)

// PointOfInterest is one column of the original "adjust" grid: a QL
// position on the curve plus Density, Shift and Contrast words.
type PointOfInterest struct {
	QL       uint16
	Density  float64 // density at that point (D3.0-style, 0..3.6)
	Shift    float64 // -100 .. +100
	Contrast float64 // 10 .. 400 (100 = neutral)
}

// Params are the user-adjustable parameters of the original dialog
// ("D3.x Adjust points"): GAMMA No., Base Color, Dmin/Dmax, the tuning
// point selection and the up-to-5 points of interest.
type Params struct {
	GammaNo   int
	BaseColor string // "clear" | "blue"
	TuningIdx int    // selected point of the curve (0..47)
	Dmin      float64
	Dmax      float64
	Points    [POICount]PointOfInterest
	Active    int // number of loaded points of interest (0..5)
}

// DefaultParams mirrors the original view-object initialization
// (sub_406580): DMAX=300 (3.00), DMIN=0, contrasts=100, shifts=0.
func DefaultParams() Params {
	return Params{
		GammaNo:   11,
		BaseColor: "clear",
		TuningIdx: 24,
		Dmin:      0,
		Dmax:      300,
		Active:    0,
	}
}

// Result holds the computed state, mirroring the document fields of the
// original binary.
type Result struct {
	GammaNo      int
	BaseColor    string
	Tuning       [TuningCount]uint16 // doc+0x18C0
	DensityWords [TuningCount]int32  // doc+0x1920 (density * 10000)
	ChangedWords [TuningCount]int32  // doc+0x8C0  "Changed Density data"
	LUT          [LutSize]uint16     // doc+0x10C0
	Weights      [TuningCount]float64
	Dmax, Dmin   float64 // doc+0x1980 / doc+0x1984
	GammaPoints  []GammaPoint
	ModeLabel    string
}

// Interp is the exact port of sub_4018E0.
func Interp(x, k1, v1, k2, v2 float64) float64 {
	return v1 + (x-k1)*(v2-v1)/(k2-k1)
}

// BuildLUT is the exact port of sub_4017A0.
// tuning: 48 tuning points; density: 48 density words (int).
// For segment s, x runs over [tuning[s-1], tuning[s]) (first segment starts
// at 0, last one includes tuning[47]); output = trunc(interp + 0.5).
func BuildLUT(tuning []int, density []int) []uint16 {
	out := make([]uint16, LutSize)
	x := 0
	for seg := 0; seg < TuningCount && x < LutSize; seg++ {
		var k1, v1 float64
		if seg > 0 {
			k1 = float64(tuning[seg-1])
			v1 = float64(density[seg-1])
		}
		k2 := float64(tuning[seg])
		v2 := float64(density[seg])
		limit := tuning[seg]
		if seg == TuningCount-1 {
			limit++
		}
		for x < limit && x < LutSize {
			v := Interp(float64(x), k1, v1, k2, v2) + 0.5
			out[x] = uint16(ftol(v))
			x++
		}
	}
	return out
}

// ftol mirrors the CRT _ftol behaviour used by the binary (truncation
// toward zero).
func ftol(v float64) int64 {
	return int64(math.Trunc(v))
}

// smoothstep is the 3t^2-2t^3 weight used by sub_40A680.
func smoothstep(t float64) float64 {
	return 3*t*t - 2*t*t*t
}

// ShiftWeights is the port of sub_40A680: it builds the 48-point weight
// curve centred on the tuning point at index idx.
//
// With d = min(P, 1023-P) the curve is w = smoothstep(t) where
//
//	t = (QL - (P-d))/d  for QL <= P  (zero when t < 0)
//	t = (P+d - QL)/d    for QL >= P
//
// This reproduces the original left branch exactly (P < 512: zero at
// QL=0 and QL=2P, one at QL=P). For P >= 512 the original adds a +1.0
// offset to the mirrored branch (a discontinuity 0 -> 2 at the mirror
// point, 1 at the pivot); this port uses the smooth mirrored decay
// instead, preserving the semantics of the left branch.
func ShiftWeights(tuning []int, idx int) []float64 {
	w := make([]float64, TuningCount)
	if idx <= 0 || idx >= TuningCount-1 {
		return w
	}
	p := tuning[idx]
	d := p
	if q := LutSize - 1 - p; q < d {
		d = q
	}
	for i, ql := range tuning {
		var t float64
		if ql <= p {
			t = float64(ql-(p-d)) / float64(d)
		} else {
			t = float64(p+d-ql) / float64(d)
		}
		if t < 0 {
			w[i] = 0
		} else {
			w[i] = smoothstep(t)
		}
	}
	return w
}

// Compute runs the full pipeline for one gamma table and parameter set.
func Compute(t GammaTable, p Params) Result {
	var res Result
	res.GammaNo = t.Number
	res.BaseColor = p.BaseColor
	res.GammaPoints = t.Points

	tuning := make([]int, TuningCount)
	dens := make([]int, TuningCount)
	for i := 0; i < TuningCount && i < len(t.Points); i++ {
		pt := t.Points[i]
		res.Tuning[i] = pt.QL
		tuning[i] = int(pt.QL)
		// fmul 10000, ftol  ->  word at 1e-4 units
		res.DensityWords[i] = int32(ftol(pt.Density * DensityScale))
		dens[i] = int(res.DensityWords[i])
	}
	for i := len(t.Points); i < TuningCount && i > 0; i++ {
		res.Tuning[i] = res.Tuning[i-1]
		res.DensityWords[i] = res.DensityWords[i-1]
		tuning[i] = tuning[i-1]
		dens[i] = dens[i-1]
	}

	changed := make([]float64, TuningCount)
	for i := 0; i < TuningCount; i++ {
		changed[i] = float64(dens[i])
	}

	// Apply the points of interest: for each loaded point, shift the curve
	// around its position and scale by contrast around its density.
	for k := 0; k < p.Active && k < POICount; k++ {
		poi := p.Points[k]
		idx := nearestPoint(tuning, int(poi.QL))
		if idx <= 0 || idx >= TuningCount-1 {
			continue
		}
		w := ShiftWeights(tuning, idx)
		if k == 0 {
			res.Weights = *(*[TuningCount]float64)(w)
		}
		anchor := poi.Density * DensityScale
		c := poi.Contrast / 100.0
		for i := 0; i < TuningCount; i++ {
			// blend: orig + shift*w (sub_40A870)
			v := changed[i] + poi.Shift*w[i]
			// contrast around the point of interest (sub_40A5D0)
			v = v*c - (c-1.0)*anchor
			changed[i] = v
		}
	}

	// Clamp to [DMIN, DMAX] and enforce a monotonic curve (sub_40A940).
	lo, hi := p.Dmin*100.0, p.Dmax*100.0
	for i := 0; i < TuningCount; i++ {
		if changed[i] < lo {
			changed[i] = lo
		}
		if changed[i] > hi {
			changed[i] = hi
		}
		res.ChangedWords[i] = int32(ftol(changed[i]))
	}
	enforceMonotonic(&res.ChangedWords)
	for i := 0; i < TuningCount; i++ {
		dens[i] = int(res.ChangedWords[i])
	}

	res.LUT = *(*[LutSize]uint16)(BuildLUT(tuning, dens))
	res.Dmax = float64(dmaxWord(res.ChangedWords)) / DensityScale
	res.Dmin = float64(dminWord(res.ChangedWords)) / DensityScale

	if p.BaseColor == "blue" {
		res.ModeLabel = "Blue"
	} else {
		res.ModeLabel = "Clear"
	}
	return res
}

// nearestPoint returns the tuning index whose QL is closest to ql.
func nearestPoint(tuning []int, ql int) int {
	best, bd := 0, math.MaxInt
	for i, v := range tuning {
		d := v - ql
		if d < 0 {
			d = -d
		}
		if d < bd {
			best, bd = i, d
		}
	}
	return best
}

// enforceMonotonic is the port of sub_40A940's ordering pass: the curve
// is forced non-decreasing (backward then forward).
func enforceMonotonic(w *[TuningCount]int32) {
	for i := TuningCount - 2; i >= 0; i-- {
		if w[i] > w[i+1] {
			w[i] = w[i+1]
		}
	}
	for i := 1; i < TuningCount; i++ {
		if w[i] < w[i-1] {
			w[i] = w[i-1]
		}
	}
}

func dmaxWord(w [TuningCount]int32) int32 {
	m := w[0]
	for _, v := range w[1:] {
		if v > m {
			m = v
		}
	}
	return m
}

func dminWord(w [TuningCount]int32) int32 {
	m := w[0]
	for _, v := range w[1:] {
		if v < m {
			m = v
		}
	}
	return m
}
