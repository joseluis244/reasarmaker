//go:build js && wasm

// Command wasm builds the SAR MAKER engine (the ocsarmk.exe
// reimplementation in internal/sar) as a WebAssembly module consumed by
// the React frontend. It exposes the engine on the global `sarWasm`
// object; every export except saveData answers with a JSON envelope
// {"ok":true,"data":...} or {"ok":false,"error":"..."}.
package main

import (
	"encoding/json"
	"syscall/js"
	"time"

	"github.com/josecamacho/reasarmaker/internal/defaults"
	"github.com/josecamacho/reasarmaker/internal/sarwire"
)

func main() {
	exports := map[string]any{
		"version": js.FuncOf(func(this js.Value, args []js.Value) any {
			info := map[string]any{
				"engine":   "sar-wasm",
				"source":   "ocsarmk.exe reimplementation (internal/sar)",
				"binary":   "PE32 x86 (FujiPhotoFilm MiyaD1 2002-10-21)",
				"sha256":   "4a6a413ceb603b9ba30fdd0921c3bf3c46050acd2ec9a7a61ab073c88338332a",
				"toolchain": "go/js-wasm",
			}
			return sarwire.MarshalEnvelope(sarwire.Envelope{OK: true, Data: mustMarshal(info)})
		}),
		"parseGamma": js.FuncOf(func(this js.Value, args []js.Value) any {
			return callJSON(args, func(text string) (any, error) {
				return sarwire.ParseGamma(text)
			})
		}),
		"defaults": js.FuncOf(func(this js.Value, args []js.Value) any {
			g, l, err := defaults.Load()
			if err != nil {
				return sarwire.MarshalEnvelope(sarwire.Envelope{Error: err.Error()})
			}
			data := map[string]any{"gamma": g, "lpmod": l}
			return sarwire.MarshalEnvelope(sarwire.Envelope{OK: true, Data: mustMarshal(data)})
		}),
		"parseLpmod": js.FuncOf(func(this js.Value, args []js.Value) any {
			return callJSON(args, func(text string) (any, error) {
				return sarwire.ParseLpmod(text)
			})
		}),
		"compute": js.FuncOf(func(this js.Value, args []js.Value) any {
			return callJSON(args, func(text string) (any, error) {
				var in sarwire.ComputeInput
				if err := json.Unmarshal([]byte(text), &in); err != nil {
					return nil, err
				}
				return sarwire.Compute(in)
			})
		}),
		// saveData(resultJSON, isoNow) returns the raw sar_savedata.txt
		// text (not an envelope). isoNow may be empty to use the current
		// time.
		"saveData": js.FuncOf(func(this js.Value, args []js.Value) any {
			if len(args) < 1 {
				return sarwire.MarshalEnvelope(sarwire.Envelope{Error: "saveData: missing result"})
			}
			var w sarwire.ResultWire
			if err := json.Unmarshal([]byte(args[0].String()), &w); err != nil {
				return sarwire.MarshalEnvelope(sarwire.Envelope{Error: err.Error()})
			}
			now := time.Now()
			if len(args) > 1 && args[1].String() != "" {
				if t, err := time.Parse(time.RFC3339Nano, args[1].String()); err == nil {
					now = t
				}
			}
			return sarwire.SaveData(w, now)
		}),
	}
	js.Global().Set("sarWasm", js.ValueOf(exports))
	select {}
}

func callJSON(args []js.Value, fn func(string) (any, error)) string {
	if len(args) < 1 {
		return sarwire.MarshalEnvelope(sarwire.Envelope{Error: "missing argument"})
	}
	v, err := fn(args[0].String())
	if err != nil {
		return sarwire.MarshalEnvelope(sarwire.Envelope{Error: err.Error()})
	}
	return sarwire.MarshalEnvelope(sarwire.Envelope{OK: true, Data: mustMarshal(v)})
}

func mustMarshal(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
