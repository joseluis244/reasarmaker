package sar

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// GammaPoint is one (QL, density) sample of a gamma table.
// In the original binary these are stored as a 16-bit QL key
// (doc+0x1ae8) and a float32 density (doc+0x2da8).
type GammaPoint struct {
	QL      uint16
	Density float64
}

// GammaTable is one "Gn" entry of gamma2.prm (e.g. G11 = "48:P:(0,0.191)...")
type GammaTable struct {
	Number int
	Points []GammaPoint
}

var (
	gammaSectionRe = regexp.MustCompile(`^G(\d+)\s*=`)
	gammaPairRe    = regexp.MustCompile(`\((\d+)\s*,\s*([0-9.]+)\)`)
)

// ParseGammaPRM parses a gamma2.prm file: lines starting with 'G'
// declare a gamma table whose quoted body contains "(ql,density)" pairs.
func ParseGammaPRM(r io.Reader) (map[int]GammaTable, error) {
	tables := map[int]GammaTable{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.HasPrefix(line, "G") {
			continue
		}
		m := gammaSectionRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		num, err := strconv.Atoi(m[1])
		if err != nil {
			return nil, fmt.Errorf("gamma2.prm line %d: bad gamma number %q", lineNo, m[1])
		}
		t := GammaTable{Number: num}
		for _, pm := range gammaPairRe.FindAllStringSubmatch(line, -1) {
			ql, err1 := strconv.Atoi(pm[1])
			d, err2 := strconv.ParseFloat(pm[2], 64)
			if err1 != nil || err2 != nil {
				return nil, fmt.Errorf("gamma2.prm line %d: bad pair %q", lineNo, pm[0])
			}
			t.Points = append(t.Points, GammaPoint{QL: uint16(ql), Density: d})
		}
		if len(t.Points) == 0 {
			return nil, fmt.Errorf("gamma2.prm line %d: G%d has no (ql,density) points", lineNo, num)
		}
		tables[num] = t
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return tables, nil
}

// SortedNumbers returns the gamma numbers in ascending order.
func SortedNumbers(tables map[int]GammaTable) []int {
	nums := make([]int, 0, len(tables))
	for n := range tables {
		nums = append(nums, n)
	}
	sort.Ints(nums)
	return nums
}
