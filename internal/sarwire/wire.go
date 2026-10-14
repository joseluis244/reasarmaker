// Package sarwire defines the JSON wire types exchanged between the React
// frontend and the WebAssembly build of the SAR MAKER engine (the
// ocsarmk.exe reimplementation in internal/sar), plus the conversion
// helpers between the wire types and the engine types.
package sarwire

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/josecamacho/reasarmaker/internal/sar"
)

// ---- wire types (camelCase JSON keys) ----

// GammaPointWire is one (ql, density) sample of a gamma table.
type GammaPointWire struct {
	QL      uint16  `json:"ql"`
	Density float64 `json:"density"`
}

// GammaTableWire is one "Gn" entry of gamma2.prm.
type GammaTableWire struct {
	Number int              `json:"number"`
	Points []GammaPointWire `json:"points"`
}

// GammaTablesWire is the parsed gamma2.prm payload: the sorted gamma
// numbers plus the per-table point lists keyed by number as a string.
type GammaTablesWire struct {
	Numbers []int                       `json:"numbers"`
	Tables  map[string][]GammaPointWire `json:"tables"`
}

// POIWire is one row of the "D3.x Adjust points" grid.
type POIWire struct {
	QL       uint16  `json:"ql"`
	Density  float64 `json:"density"`
	Shift    float64 `json:"shift"`
	Contrast float64 `json:"contrast"`
}

// ParamsWire mirrors sar.Params. The points slice already contains only
// the active rows (rows with an empty QL are dropped by the frontend).
type ParamsWire struct {
	GammaNo   int       `json:"gammaNo"`
	BaseColor string    `json:"baseColor"`
	TuningIdx int       `json:"tuningIdx"`
	Dmin      float64   `json:"dmin"`
	Dmax      float64   `json:"dmax"`
	Points    []POIWire `json:"points"`
}

// ComputeInput is the payload of the compute export.
type ComputeInput struct {
	Table  GammaTableWire `json:"table"`
	Params ParamsWire     `json:"params"`
}

// ResultWire mirrors sar.Result with JSON-friendly slices.
type ResultWire struct {
	GammaNo      int       `json:"gammaNo"`
	BaseColor    string    `json:"baseColor"`
	Tuning       []uint16  `json:"tuning"`
	DensityWords []int32   `json:"densityWords"`
	ChangedWords []int32   `json:"changedWords"`
	LUT          []uint16  `json:"lut"`
	Weights      []float64 `json:"weights"`
	Dmax         float64   `json:"dmax"`
	Dmin         float64   `json:"dmin"`
	ModeLabel    string    `json:"modeLabel"`
}

// Envelope wraps every wasm export result (data on success, error text
// otherwise).
type Envelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}

// MarshalEnvelope renders an Envelope as JSON text.
func MarshalEnvelope(e Envelope) string {
	b, err := json.Marshal(e)
	if err != nil {
		return fmt.Sprintf(`{"ok":false,"error":%q}`, err.Error())
	}
	return string(b)
}

func okEnvelope(v any) Envelope {
	b, err := json.Marshal(v)
	if err != nil {
		return Envelope{OK: false, Error: err.Error()}
	}
	return Envelope{OK: true, Data: b}
}

func errEnvelope(err error) Envelope {
	return Envelope{OK: false, Error: err.Error()}
}

// ---- conversions ----

// ParseGamma parses a gamma2.prm text into the wire payload.
func ParseGamma(text string) (GammaTablesWire, error) {
	tables, err := sar.ParseGammaPRM(strings.NewReader(text))
	if err != nil {
		return GammaTablesWire{}, err
	}
	numbers := sar.SortedNumbers(tables)
	w := GammaTablesWire{Numbers: numbers, Tables: map[string][]GammaPointWire{}}
	for n, t := range tables {
		pts := make([]GammaPointWire, 0, len(t.Points))
		for _, p := range t.Points {
			pts = append(pts, GammaPointWire{QL: p.QL, Density: p.Density})
		}
		w.Tables[fmt.Sprintf("%d", n)] = pts
	}
	return w, nil
}

// ParseLpmod parses an LPMOD.PRM text and returns the sorted section names.
func ParseLpmod(text string) ([]string, error) {
	luts, err := sar.ParseLPModPRM(strings.NewReader(text))
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(luts))
	for name := range luts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// Compute runs the full SAR pipeline for one gamma table and parameter
// set, converting between the wire types and internal/sar.
func Compute(in ComputeInput) (ResultWire, error) {
	if len(in.Table.Points) == 0 {
		return ResultWire{}, fmt.Errorf("Check GAMMA Table: G%d has no points", in.Table.Number)
	}
	t := sar.GammaTable{Number: in.Table.Number, Points: make([]sar.GammaPoint, 0, len(in.Table.Points))}
	for _, p := range in.Table.Points {
		t.Points = append(t.Points, sar.GammaPoint{QL: p.QL, Density: p.Density})
	}

	p := sar.DefaultParams()
	p.GammaNo = in.Params.GammaNo
	if in.Params.BaseColor != "" {
		p.BaseColor = in.Params.BaseColor
	}
	p.TuningIdx = in.Params.TuningIdx
	p.Dmin = in.Params.Dmin
	p.Dmax = in.Params.Dmax
	active := len(in.Params.Points)
	if active > sar.POICount {
		active = sar.POICount
	}
	for k := 0; k < active; k++ {
		pw := in.Params.Points[k]
		p.Points[k] = sar.PointOfInterest{
			QL:       pw.QL,
			Density:  pw.Density,
			Shift:    pw.Shift,
			Contrast: pw.Contrast,
		}
	}
	p.Active = active

	return FromResult(sar.Compute(t, p)), nil
}

// FromResult converts an engine result into its wire form.
func FromResult(r sar.Result) ResultWire {
	return ResultWire{
		GammaNo:      r.GammaNo,
		BaseColor:    r.BaseColor,
		Tuning:       append([]uint16(nil), r.Tuning[:]...),
		DensityWords: append([]int32(nil), r.DensityWords[:]...),
		ChangedWords: append([]int32(nil), r.ChangedWords[:]...),
		LUT:          append([]uint16(nil), r.LUT[:]...),
		Weights:      append([]float64(nil), r.Weights[:]...),
		Dmax:         r.Dmax,
		Dmin:         r.Dmin,
		ModeLabel:    r.ModeLabel,
	}
}

// toResult converts a wire result back into an engine result (used by
// SaveData, which only reads the fields it prints).
func toResult(w ResultWire) sar.Result {
	var r sar.Result
	r.GammaNo = w.GammaNo
	r.BaseColor = w.BaseColor
	copy(r.Tuning[:], w.Tuning)
	copy(r.DensityWords[:], w.DensityWords)
	copy(r.ChangedWords[:], w.ChangedWords)
	copy(r.LUT[:], w.LUT)
	copy(r.Weights[:], w.Weights)
	r.Dmax, r.Dmin = w.Dmax, w.Dmin
	r.ModeLabel = w.ModeLabel
	return r
}

// SaveData renders sar_savedata.txt for a computed result at the given
// instant (the original save routine stamps the current date/time).
func SaveData(w ResultWire, now time.Time) string {
	return sar.SaveData(toResult(w), now)
}
