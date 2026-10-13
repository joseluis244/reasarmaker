package sar

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// LUT is one "$LPn_xxx" section of LPMOD.PRM: 301 (QL -> luminance)
// mappings where QL runs from 0 to 300 and the value up to 1023.
type LUT struct {
	Name    string
	Entries [301]uint16
}

var (
	lutSectionRe = regexp.MustCompile(`^\$([A-Za-z0-9_]+)\s*:`)
	lutPairRe    = regexp.MustCompile(`(\d+)\s*[,.]\s*(\d+)`)
)

// ParseLPModPRM parses LPMOD.PRM. Each section is "$NAME:" followed by
// "key,value:" pairs separated by ':' or ';' (per the file's own header
// comment). Pairs outside the expected 0..300 key range are ignored.
func ParseLPModPRM(r io.Reader) (map[string]LUT, error) {
	luts := map[string]LUT{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	var cur *LUT
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "$END") {
			if cur != nil {
				luts[cur.Name] = *cur
			}
			cur = nil
			continue
		}
		if m := lutSectionRe.FindStringSubmatch(line); m != nil {
			if cur != nil {
				luts[cur.Name] = *cur
			}
			l := LUT{Name: m[1]}
			cur = &l
			// a section may declare pairs on the same line after the colon
			line = line[len(m[0]):]
		}
		if cur == nil {
			continue
		}
		for _, pm := range lutPairRe.FindAllStringSubmatch(line, -1) {
			key := 0
			val := 0
			fmt.Sscanf(pm[1], "%d", &key)
			fmt.Sscanf(pm[2], "%d", &val)
			if key >= 0 && key <= 300 {
				cur.Entries[key] = uint16(val)
			}
		}
	}
	if cur != nil {
		luts[cur.Name] = *cur
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(luts) == 0 {
		return nil, fmt.Errorf("no $LP sections found")
	}
	return luts, nil
}

// Lookup interpolates the LUT at a QL value (0..300). Out-of-range values
// are clamped to the first/last entries.
func (l *LUT) Lookup(ql float64) float64 {
	if ql <= 0 {
		return float64(l.Entries[0])
	}
	if ql >= 300 {
		return float64(l.Entries[300])
	}
	lo := int(ql)
	hi := lo + 1
	frac := ql - float64(lo)
	v0 := float64(l.Entries[lo])
	v1 := float64(l.Entries[hi])
	return v0 + (v1-v0)*frac
}
