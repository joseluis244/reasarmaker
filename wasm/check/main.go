// Command check evaluates the wasm smoke-test cases with the native
// (non-wasm) build of the engine and prints the resulting JSON array to
// stdout. wasm/smoke.mjs runs the same cases through the compiled wasm
// module and asserts both outputs are identical.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/josecamacho/reasarmaker/internal/defaults"
	"github.com/josecamacho/reasarmaker/internal/sarwire"
)

type op struct {
	Op        string          `json:"op"`
	File      string          `json:"file,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	UseResult int             `json:"useResult,omitempty"`
	Now       string          `json:"now,omitempty"`
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "check:", err)
		os.Exit(1)
	}
}

func main() {
	casesPath := filepath.Join("wasm", "testdata", "cases.json")
	if len(os.Args) > 1 {
		casesPath = os.Args[1]
	}
	data, err := os.ReadFile(casesPath)
	must(err)
	var cases []op
	must(json.Unmarshal(data, &cases))

	results := make([]any, 0, len(cases))
	computed := map[int]sarwire.ResultWire{}
	for _, c := range cases {
		switch c.Op {
		case "defaults":
			g, l, err := defaults.Load()
			must(err)
			results = append(results, sarwire.Envelope{OK: true, Data: mustMarshal(map[string]any{"gamma": g, "lpmod": l})})
		case "parseGamma":
			text, err := os.ReadFile(filepath.FromSlash(c.File))
			must(err)
			v, err := sarwire.ParseGamma(string(text))
			must(err)
			results = append(results, sarwire.Envelope{OK: true, Data: mustMarshal(v)})
		case "parseLpmod":
			text, err := os.ReadFile(filepath.FromSlash(c.File))
			must(err)
			v, err := sarwire.ParseLpmod(string(text))
			must(err)
			results = append(results, sarwire.Envelope{OK: true, Data: mustMarshal(v)})
		case "compute":
			var in sarwire.ComputeInput
			must(json.Unmarshal(c.Input, &in))
			v, err := sarwire.Compute(in)
			must(err)
			computed[len(results)] = v
			results = append(results, sarwire.Envelope{OK: true, Data: mustMarshal(v)})
		case "saveData":
			v, ok := computed[c.UseResult]
			if !ok {
				must(fmt.Errorf("saveData: useResult %d does not reference a compute result", c.UseResult))
			}
			now := time.Now()
			if c.Now != "" {
				now, err = time.Parse(time.RFC3339Nano, c.Now)
				must(err)
			}
			results = append(results, sarwire.SaveData(v, now))
		default:
			must(fmt.Errorf("unknown op %q", c.Op))
		}
	}
	out, err := json.Marshal(results)
	must(err)
	fmt.Println(string(out))
}

func mustMarshal(v any) json.RawMessage {
	b, err := json.Marshal(v)
	must(err)
	return b
}
