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

func computeSymbolMetrics(idx *core.Index) []SymbolMetrics {
	metrics := map[SymbolID]*SymbolMetrics{}
	for _, sym := range idx.Symbols {
		if sym.ID == "" {
			continue
		}
		metrics[sym.ID] = &SymbolMetrics{
			SymbolID:      sym.ID,
			UnitID:        sym.UnitID,
			Kind:          sym.Kind,
			Name:          sym.Name,
			QualifiedName: sym.QualifiedName,
			Location:      sym.Location,
		}
	}
	for _, occ := range idx.Occurrences {
		if occ.Kind != OccurrenceReference && occ.Kind != OccurrenceRead && occ.Kind != OccurrenceWrite && occ.Kind != OccurrenceDoc {
			continue
		}
		m := ensureSymbolMetric(metrics, idx, occ.SymbolID)
		if m == nil {
			continue
		}
		m.ReferenceCount++
	}
	for _, edge := range idx.Edges {
		switch edge.Kind {
		case EdgeCalls:
			if m := ensureSymbolMetric(metrics, idx, SymbolID(edge.From)); m != nil {
				m.CallFanOut++
			}
			if m := ensureSymbolMetric(metrics, idx, SymbolID(edge.To)); m != nil {
				m.CallFanIn++
			}
		case EdgeImplements:
			if m := ensureSymbolMetric(metrics, idx, SymbolID(edge.From)); m != nil {
				m.ImplementationCount++
			}
		}
	}
	var out []SymbolMetrics
	for _, m := range metrics {
		m.PressureScore = float64(m.ReferenceCount + 2*m.CallFanIn + m.CallFanOut + m.ImplementationCount)
		if m.PressureScore > 0 {
			m.Evidence = []Evidence{{Kind: "symbol_pressure_score", Message: "score is based on references, call fan-in/out, and implementation edges"}}
		}
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.PressureScore != b.PressureScore {
			return a.PressureScore > b.PressureScore
		}
		if a.Location.URI != b.Location.URI {
			return a.Location.URI < b.Location.URI
		}
		return a.Location.Range.Start.Offset < b.Location.Range.Start.Offset
	})
	return out
}

func ensureSymbolMetric(metrics map[SymbolID]*SymbolMetrics, idx *core.Index, id SymbolID) *SymbolMetrics {
	if id == "" {
		return nil
	}
	if m, ok := metrics[id]; ok {
		return m
	}
	sym, ok := idx.ByID[id]
	if !ok {
		return nil
	}
	m := &SymbolMetrics{
		SymbolID:      sym.ID,
		UnitID:        sym.UnitID,
		Kind:          sym.Kind,
		Name:          sym.Name,
		QualifiedName: sym.QualifiedName,
		Location:      sym.Location,
	}
	metrics[id] = m
	return m
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
