// Package defaults embeds the sample calibration files (gamma2.prm and
// LPMOD.PRM) into every build — native and wasm — so the app starts with
// the ocsarmk.exe QA data already loaded, without uploading anything.
package defaults

import (
	_ "embed"

	"github.com/josecamacho/reasarmaker/internal/sarwire"
)

//go:embed gamma2.prm
var gammaPRM string

//go:embed lpmod.prm
var lpmodPRM string

// GammaPRM returns the embedded gamma2.prm text.
func GammaPRM() string { return gammaPRM }

// LpmodPRM returns the embedded LPMOD.PRM text.
func LpmodPRM() string { return lpmodPRM }

// Load parses the embedded files and returns the same payloads the
// parseGamma / parseLpmod wasm exports produce. The embedded files are
// valid samples, so errors here are programmer errors.
func Load() (gamma sarwire.GammaTablesWire, lpmod []string, err error) {
	gamma, err = sarwire.ParseGamma(gammaPRM)
	if err != nil {
		return gamma, nil, err
	}
	lpmod, err = sarwire.ParseLpmod(lpmodPRM)
	return gamma, lpmod, err
}
