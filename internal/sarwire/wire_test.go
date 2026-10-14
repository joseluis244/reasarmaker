package sarwire

import (
	"strings"
	"testing"
	"time"

	"github.com/josecamacho/reasarmaker/internal/sar"
)

func syntheticTable() GammaTableWire {
	t := GammaTableWire{Number: 11}
	for i := 0; i < sar.TuningCount; i++ {
		t.Points = append(t.Points, GammaPointWire{
			QL:      uint16(i * 20),
			Density: 0.5 + float64(i)*0.05,
		})
	}
	return t
}

func TestComputeMatchesEngine(t *testing.T) {
	in := ComputeInput{
		Table: syntheticTable(),
		Params: ParamsWire{
			GammaNo:   11,
			BaseColor: "clear",
			TuningIdx: 24,
			Dmin:      0,
			Dmax:      300,
			Points: []POIWire{
				{QL: 200, Density: 1.0, Shift: 50, Contrast: 100},
			},
		},
	}
	got, err := Compute(in)
	if err != nil {
		t.Fatal(err)
	}

	// Reference run directly against the engine.
	var tab sar.GammaTable
	tab.Number = 11
	for _, p := range in.Table.Points {
		tab.Points = append(tab.Points, sar.GammaPoint{QL: p.QL, Density: p.Density})
	}
	p := sar.DefaultParams()
	p.Points[0] = sar.PointOfInterest{QL: 200, Density: 1.0, Shift: 50, Contrast: 100}
	p.Active = 1
	want := sar.Compute(tab, p)

	if len(got.Tuning) != sar.TuningCount {
		t.Fatalf("tuning len = %d", len(got.Tuning))
	}
	for i := 0; i < sar.TuningCount; i++ {
		if got.Tuning[i] != want.Tuning[i] {
			t.Fatalf("tuning[%d] = %d, want %d", i, got.Tuning[i], want.Tuning[i])
		}
		if got.DensityWords[i] != want.DensityWords[i] {
			t.Fatalf("density[%d] = %d, want %d", i, got.DensityWords[i], want.DensityWords[i])
		}
		if got.ChangedWords[i] != want.ChangedWords[i] {
			t.Fatalf("changed[%d] = %d, want %d", i, got.ChangedWords[i], want.ChangedWords[i])
		}
		if got.Weights[i] != want.Weights[i] {
			t.Fatalf("weight[%d] = %v, want %v", i, got.Weights[i], want.Weights[i])
		}
	}
	if got.Dmax != want.Dmax || got.Dmin != want.Dmin {
		t.Fatalf("dmax/dmin = %v/%v, want %v/%v", got.Dmax, got.Dmin, want.Dmax, want.Dmin)
	}
	if got.ModeLabel != "Clear" {
		t.Fatalf("mode = %q", got.ModeLabel)
	}
}

func TestParseGammaInline(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("G11 = \"48:P:(0,0.191)(22,0.2)(44,0.25)\"\n")
	sb.WriteString("G47 = \"48:P:(0,0.9)(22,1.0)\"\n")
	got, err := ParseGamma(sb.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Numbers) != 2 || got.Numbers[0] != 11 || got.Numbers[1] != 47 {
		t.Fatalf("numbers = %v", got.Numbers)
	}
	if len(got.Tables["11"]) != 3 || len(got.Tables["47"]) != 2 {
		t.Fatalf("tables = %v", got.Tables)
	}
}

func TestParseLpmodInline(t *testing.T) {
	got, err := ParseLpmod("$LP1_CLEAR: 0,0: 1,10: 300,1023: $END\n$LP2_BLUE: 0,5: 1,8: $END\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "LP1_CLEAR" || got[1] != "LP2_BLUE" {
		t.Fatalf("sections = %v", got)
	}
}

func TestSaveDataFixedTime(t *testing.T) {
	in := ComputeInput{Table: syntheticTable(), Params: ParamsWire{GammaNo: 11, Dmax: 300, TuningIdx: 24}}
	res, err := Compute(in)
	if err != nil {
		t.Fatal(err)
	}
	out := SaveData(res, time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC))
	for _, want := range []string{
		"2026-10-13 12:00:00",
		"GAMMA NO :11",
		"dmax    dmin    density    shift   contrast",
		"Changed Density data (48 points)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("SaveData output missing %q:\n%s", want, out)
		}
	}
}
