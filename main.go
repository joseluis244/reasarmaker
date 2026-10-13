package main

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/josecamacho/reasarmaker/internal/sar"
)

//go:embed web/*
var webFS embed.FS

type App struct {
	mu     sync.Mutex
	luts   map[string]sar.LUT
	gammas map[int]sar.GammaTable
	result *sar.Result
	params sar.Params
	loaded bool
}

func (a *App) parseFiles(r *http.Request) error {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return fmt.Errorf("parse upload: %w", err)
	}
	luts := map[string]sar.LUT{}
	gammas := map[int]sar.GammaTable{}
	for _, key := range []string{"lpmod", "gamma"} {
		f, _, err := r.FormFile(key)
		if err != nil {
			continue
		}
		data, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			return fmt.Errorf("read %s: %w", key, err)
		}
		if key == "lpmod" {
			l, err := sar.ParseLPModPRM(strings.NewReader(string(data)))
			if err != nil {
				return fmt.Errorf("LPMOD.PRM: %w", err)
			}
			luts = l
		} else {
			g, err := sar.ParseGammaPRM(strings.NewReader(string(data)))
			if err != nil {
				return fmt.Errorf("gamma2.prm: %w", err)
			}
			gammas = g
		}
	}
	if len(luts) == 0 && len(gammas) == 0 {
		return fmt.Errorf("no LPMOD.PRM / gamma2.prm file received")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(luts) > 0 {
		a.luts = luts
	}
	if len(gammas) > 0 {
		a.gammas = gammas
		if p, ok := sar.SortedNumbers(gammas)[0], true; ok && a.params.GammaNo == 0 {
			_ = p
		}
	}
	a.loaded = true
	return nil
}

func main() {
	app := &App{params: sar.DefaultParams()}
	funcs := template.FuncMap{
		"seq": func(a, b int) []int {
			s := make([]int, 0, b-a+1)
			for i := a; i <= b; i++ {
				s = append(s, i)
			}
			return s
		},
		"add": func(a, b int) int { return a + b },
	}
	tmpl, err := template.New("web").Funcs(funcs).ParseFS(webFS, "web/*.html")
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		app.mu.Lock()
		data := map[string]any{
			"Loaded": app.loaded,
			"Gammas": sar.SortedNumbers(app.gammas),
			"Params": app.params,
			"LUTs":   sortedKeys(app.luts),
		}
		app.mu.Unlock()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
			http.Error(w, err.Error(), 500)
		}
	})

	mux.HandleFunc("POST /upload", func(w http.ResponseWriter, r *http.Request) {
		if err := app.parseFiles(r); err != nil {
			w.WriteHeader(400)
			fmt.Fprintf(w, `<div class="status error">%s</div>`, template.HTMLEscapeString(err.Error()))
			return
		}
		app.mu.Lock()
		defer app.mu.Unlock()
		var b strings.Builder
		fmt.Fprintf(&b, `<div class="status ok">Loaded: %d gamma tables`, len(app.gammas))
		if len(app.luts) > 0 {
			fmt.Fprintf(&b, ", %d LPMOD sections", len(app.luts))
		}
		b.WriteString(`</div>`)
		b.WriteString(`<div id="gamma-select" hx-swap-oob="innerHTML">`)
		b.WriteString(renderGammaSelect(app))
		b.WriteString(`</div>`)
		fmt.Fprint(w, b.String())
	})

	mux.HandleFunc("POST /compute", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", 400)
			return
		}
		p := sar.DefaultParams()
		p.GammaNo = atoiDefault(r.FormValue("gamma_no"), 11)
		p.BaseColor = r.FormValue("base_color")
		if p.BaseColor == "" {
			p.BaseColor = "clear"
		}
		p.TuningIdx = atoiDefault(r.FormValue("tuning_idx"), 24)
		p.Dmin = atofDefault(r.FormValue("dmin"), 0)
		p.Dmax = atofDefault(r.FormValue("dmax"), 300)

		active := 0
		for k := 0; k < sar.POICount; k++ {
			pre := fmt.Sprintf("p%d_", k)
			ql := r.FormValue(pre + "ql")
			if strings.TrimSpace(ql) == "" {
				continue
			}
			poi := sar.PointOfInterest{
				QL:       uint16(atoiDefault(ql, 0)),
				Density:  atofDefault(r.FormValue(pre+"density"), 0),
				Shift:    atofDefault(r.FormValue(pre+"shift"), 0),
				Contrast: atofDefault(r.FormValue(pre+"contrast"), 100),
			}
			p.Points[active] = poi
			active++
		}
		p.Active = active

		app.mu.Lock()
		t, ok := app.gammas[p.GammaNo]
		app.mu.Unlock()
		if !ok {
			fmt.Fprint(w, `<div class="status error">Check GAMMA Table: G`+strconv.Itoa(p.GammaNo)+` not found. Load gamma2.prm first.</div>`)
			return
		}
		res := sar.Compute(t, p)
		app.mu.Lock()
		app.result = &res
		app.params = p
		app.mu.Unlock()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, renderResults(res))
	})

	mux.HandleFunc("GET /download", func(w http.ResponseWriter, r *http.Request) {
		app.mu.Lock()
		res := app.result
		app.mu.Unlock()
		if res == nil {
			http.Error(w, "compute first", 400)
			return
		}
		body := sar.SaveData(*res, time.Now())
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="sar_savedata.txt"`)
		io.WriteString(w, body)
	})

	addr := ":8080"
	log.Printf("SAR MAKER (Go) listening on http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func atoiDefault(s string, def int) int {
	if v, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
		return v
	}
	return def
}

func atofDefault(s string, def float64) float64 {
	if v, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
		return v
	}
	return def
}

func renderGammaSelect(a *App) string {
	var b strings.Builder
	b.WriteString(`<label for="gamma_no">GAMMA No.</label><select id="gamma_no" name="gamma_no">`)
	for _, n := range sar.SortedNumbers(a.gammas) {
		sel := ""
		if n == a.params.GammaNo {
			sel = " selected"
		}
		fmt.Fprintf(&b, `<option value="%d"%s>G%d</option>`, n, sel, n)
	}
	b.WriteString(`</select>`)
	return b.String()
}

func renderResults(res sar.Result) string {
	var b strings.Builder
	b.WriteString(`<div id="results">`)
	b.WriteString(`<div class="status ok">save complete</div>`)
	fmt.Fprintf(&b, `<h2>SAR MAKER for Win — Mode %s</h2>`, res.ModeLabel)
	fmt.Fprintf(&b, `<p>GAMMA NO : %d — dmax %3.2f, dmin %3.2f</p>`, res.GammaNo, res.Dmax, res.Dmin)
	b.WriteString(`<p><a href="/download" download="sar_savedata.txt" class="btn">Download sar_savedata.txt</a></p>`)
	b.WriteString(`<div class="cols">`)
	b.WriteString(`<table><thead><tr><th>#</th><th>QL</th><th>Density</th><th>Changed</th><th>Δ</th><th>weight</th></tr></thead><tbody>`)
	for i := 0; i < sar.TuningCount; i++ {
		d := float64(res.DensityWords[i]) / sar.DensityScale
		c := float64(res.ChangedWords[i]) / sar.DensityScale
		delta := c - d
		fmt.Fprintf(&b, `<tr><td>%d</td><td>%d</td><td>%3.4f</td><td>%3.4f</td><td>%+3.4f</td><td>%5.3f</td></tr>`,
			i, res.Tuning[i], d, c, delta, res.Weights[i])
	}
	b.WriteString(`</tbody></table>`)
	b.WriteString(renderSVG(res))
	b.WriteString(`</div></div>`)
	return b.String()
}

func renderSVG(res sar.Result) string {
	w, h := 520.0, 260.0
	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="0 0 %.0f %.0f" class="chart">`, w, h)
	b.WriteString(`<rect width="100%" height="100%" fill="#0b0f14"/>`)
	for _, d := range []float64{0, 0.5, 1.0, 1.5, 2.0, 2.5, 3.0, 3.5} {
		y := h - 20 - (d/3.5)*(h-40)
		fmt.Fprintf(&b, `<line x1="40" y1="%.1f" x2="%.0f" y2="%.1f" stroke="#22303c"/>`, y, w-8, y)
		fmt.Fprintf(&b, `<text x="6" y="%.1f" fill="#8aa0b4" font-size="10">%3.1f</text>`, y+3, d)
	}
	curve := func(words [sar.TuningCount]int32, stroke string) string {
		pts := []string{}
		for i := 0; i < sar.TuningCount; i++ {
			x := 40 + float64(res.Tuning[i])/1023*(w-48)
			y := h - 20 - (float64(words[i])/sar.DensityScale/3.5)*(h-40)
			pts = append(pts, fmt.Sprintf("%.1f,%.1f", x, y))
		}
		return `<polyline points="` + strings.Join(pts, " ") + `" fill="none" stroke="` + stroke + `" stroke-width="1.5"/>`
	}
	b.WriteString(curve(res.DensityWords, "#5a7f96"))
	b.WriteString(curve(res.ChangedWords, "#4cc38a"))
	// shift weight overlay (scaled to 0..3.5)
	pts := []string{}
	for i := 0; i < sar.TuningCount; i++ {
		x := 40 + float64(res.Tuning[i])/1023*(w-48)
		wv := res.Weights[i]
		if wv > 1 {
			wv = 1
		}
		y := h - 20 - (wv/3.5)*(h-40)
		pts = append(pts, fmt.Sprintf("%.1f,%.1f", x, y))
	}
	b.WriteString(`<polyline points="` + strings.Join(pts, " ") + `" fill="none" stroke="#ffb454" stroke-width="1" stroke-dasharray="3 3"/>`)
	b.WriteString(`<text x="10" y="14" fill="#5a7f96" font-size="10">before</text>`)
	b.WriteString(`<text x="60" y="14" fill="#4cc38a" font-size="10">after</text>`)
	b.WriteString(`<text x="104" y="14" fill="#ffb454" font-size="10">shift weight</text>`)
	b.WriteString(`</svg>`)
	return b.String()
}
