package sar

import (
	"fmt"
	"strings"
	"time"
)

// SaveData renders the sar_savedata.txt content, reproducing the structure
// observed in the original save routine (sub_405700):
//
//	----------------------------------------------------------------
//	YYYY-MM-DD HH:MM:SS
//	GAMMA NO :<n>
//	    dmax    dmin    density    shift   contrast
//	<per-point rows>
//	Changed Density data (48 points)
//	<48 density values>
//
// Density values are printed from the 1e-4 fixed-point words as %3.2f.
func SaveData(res Result, now time.Time) string {
	var b strings.Builder
	b.WriteString(strings.Repeat("-", 64))
	b.WriteString("\n")
	b.WriteString(now.Format("2006-01-02 15:04:05"))
	b.WriteString("\n")
	fmt.Fprintf(&b, "GAMMA NO :%d\n", res.GammaNo)
	b.WriteString("    dmax    dmin    density    shift   contrast\n")

	// Per-point rows: (tuning, density) pair, mirroring the gamma2.prm
	// "(ql,density)" convention the original rows are built from. Both
	// density values come from the adjusted curve (doc+0x860).
	for i := 0; i < TuningCount; i++ {
		c := float64(res.ChangedWords[i]) / DensityScale
		fmt.Fprintf(&b, "( %4d, %3.2f) : %3.2f\n", res.Tuning[i], c, c)
	}

	b.WriteString("Changed Density data (48 points)\n")
	for i := 0; i < TuningCount; i++ {
		fmt.Fprintf(&b, "%3.2f", float64(res.ChangedWords[i])/DensityScale)
		if (i+1)%8 == 0 || i == TuningCount-1 {
			b.WriteString("\n")
		} else {
			b.WriteString(" ")
		}
	}
	return b.String()
}
