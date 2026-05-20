package editor

import (
	"sort"
	"strings"

	"github.com/codewandler/editor/internal/core"
)

func computeMetrics(idx *core.Index) []UnitMetrics {
	metrics := map[string]*UnitMetrics{}
	for unit, files := range idx.UnitFiles {
		metrics[unit] = &UnitMetrics{UnitID: unit, FileCount: len(files)}
		for _, p := range files {
			metrics[unit].LOC += idx.FileLOC[p]
		}
	}
	for _, imp := range idx.Imports {
		m := ensureUnitMetric(metrics, imp.FromUnit)
		m.DirectFanOut++
		for unit := range metrics {
			if strings.HasSuffix(imp.Import, packageDir(unit)) {
				metrics[unit].DirectFanIn++
			}
		}
	}
	for _, sym := range idx.Symbols {
		m := ensureUnitMetric(metrics, sym.UnitID)
		if isExported(sym.Name) {
			m.PublicSymbolCount++
		}
		if sym.Kind == SymbolInterface {
			m.InterfaceCount++
		}
	}
	for _, edge := range idx.Edges {
		if edge.Kind == EdgeCalls {
			if from, ok := idx.ByID[SymbolID(edge.From)]; ok {
				ensureUnitMetric(metrics, from.UnitID).CallFanOut++
				ensureUnitMetric(metrics, from.UnitID).SymbolFanOut++
			}
			if to, ok := idx.ByID[SymbolID(edge.To)]; ok {
				ensureUnitMetric(metrics, to.UnitID).CallFanIn++
				ensureUnitMetric(metrics, to.UnitID).SymbolFanIn++
			}
		}
		if edge.Kind == EdgeImplements {
			if from, ok := idx.ByID[SymbolID(edge.From)]; ok {
				ensureUnitMetric(metrics, from.UnitID).ImplementationCount++
			}
		}
	}
	var out []UnitMetrics
	for _, m := range metrics {
		m.PressureScore = float64(3*m.DirectFanIn + 2*m.CallFanIn + m.PublicSymbolCount + m.FileCount + m.ImplementationCount)
		m.Evidence = []Evidence{{Kind: "pressure_score", Message: "score is based on fan-in, call fan-in, public symbols, file count, and implementations"}}
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UnitID < out[j].UnitID
	})
	return out
}

func ensureUnitMetric(metrics map[string]*UnitMetrics, unit string) *UnitMetrics {
	m, ok := metrics[unit]
	if !ok {
		m = &UnitMetrics{UnitID: unit}
		metrics[unit] = m
	}
	return m
}

func packageDir(unit string) string {
	if i := strings.IndexByte(unit, '#'); i >= 0 {
		return unit[:i]
	}
	return unit
}

func isExported(name string) bool {
	if name == "" {
		return false
	}
	r := []rune(name)[0]
	return r >= 'A' && r <= 'Z'
}
