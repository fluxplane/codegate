package goast

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
)

func (b GoBackend) Suggest(ctx context.Context, snapshot Snapshot, scope Scope) ([]Proposal, error) {
	idx, err := buildIndex(ctx, snapshot, scope)
	if err != nil {
		return nil, err
	}
	var proposals []Proposal
	unused, err := suggestUnusedPrivate(ctx, snapshot, idx)
	if err != nil {
		return nil, err
	}
	proposals = append(proposals, unused...)
	proposals = append(proposals, suggestLargeFunctions(idx)...)
	proposals = append(proposals, suggestLargeParameterLists(idx)...)
	proposals = append(proposals, suggestBooleanFlags(idx)...)
	proposals = append(proposals, suggestHighPressureSymbols(idx)...)
	proposals = append(proposals, suggestHighFanIn(idx)...)
	for i := range proposals {
		proposals[i].ID = fmt.Sprintf("prop_%03d", i+1)
	}
	return proposals, nil
}

func suggestUnusedPrivate(ctx context.Context, snapshot Snapshot, idx *index) ([]Proposal, error) {
	used := map[SymbolID]bool{}
	for _, occ := range idx.occurrences {
		if !isUsageOccurrence(occ.Kind) {
			continue
		}
		used[occ.SymbolID] = true
	}
	var out []Proposal
	for _, sym := range idx.symbols {
		if sym.Kind == SymbolField || sym.Kind == SymbolMethod || sym.Kind == SymbolPackage || isExported(sym.Name) || used[sym.ID] {
			continue
		}
		if strings.HasPrefix(sym.Name, "_") || isGoEntrypoint(sym) {
			continue
		}
		hash, err := sourceHash(ctx, snapshot, sym)
		if err != nil {
			return nil, err
		}
		out = append(out, Proposal{
			Kind:       RefactorDeleteSymbol,
			Title:      "Delete unused private symbol",
			Summary:    fmt.Sprintf("Private %s %q has no AST-detected references.", sym.Kind, sym.QualifiedName),
			Confidence: MediumConfidence,
			Risk:       RiskLow,
			Targets:    []Symbol{sym},
			Evidence:   []Evidence{{Kind: "unused_private_symbol", Message: "No references were found by the AST backend.", Location: sym.Location}},
			Operations: []Operation{DeleteSymbol{Target: SymbolSelector{ID: sym.ID}, ExpectedHash: hash}},
			Metrics:    map[string]float64{"references": 0},
		})
	}
	return out, nil
}

func isUsageOccurrence(kind OccurrenceKind) bool {
	switch kind {
	case OccurrenceReference, OccurrenceRead, OccurrenceWrite, OccurrenceCall:
		return true
	default:
		return false
	}
}

func isGoEntrypoint(sym Symbol) bool {
	if sym.Kind != SymbolFunction {
		return false
	}
	if sym.Name == "init" {
		return true
	}
	return sym.Name == "main" && (sym.UnitID == "main" || strings.HasSuffix(sym.UnitID, "#main"))
}

func suggestLargeFunctions(idx *index) []Proposal {
	var out []Proposal
	for _, sym := range idx.symbols {
		if sym.Kind != SymbolFunction && sym.Kind != SymbolMethod {
			continue
		}
		lines := sym.Location.Range.End.Line - sym.Location.Range.Start.Line + 1
		if lines < 40 {
			continue
		}
		out = append(out, Proposal{
			Kind:       RefactorExtractFunction,
			Title:      "Split large function",
			Summary:    fmt.Sprintf("%q spans %d lines.", sym.QualifiedName, lines),
			Confidence: MediumConfidence,
			Risk:       RiskMedium,
			Targets:    []Symbol{sym},
			Evidence: []Evidence{
				{Kind: "large_function", Message: "Function length exceeds 40 lines.", Location: sym.Location, Metrics: map[string]float64{"lines": float64(lines)}},
				advisoryNoOperationEvidence("Extraction range and replacement call cannot be inferred safely by the AST backend."),
			},
			Metrics: map[string]float64{"lines": float64(lines)},
		})
	}
	return out
}

func suggestLargeParameterLists(idx *index) []Proposal {
	var out []Proposal
	for _, sym := range idx.symbols {
		if sym.Kind != SymbolFunction && sym.Kind != SymbolMethod {
			continue
		}
		n := countParams(sym.Signature)
		if n < 5 {
			continue
		}
		out = append(out, Proposal{
			Kind:       RefactorIntroduceConfig,
			Title:      "Introduce parameter object",
			Summary:    fmt.Sprintf("%q has %d parameters.", sym.QualifiedName, n),
			Confidence: MediumConfidence,
			Risk:       RiskMedium,
			Targets:    []Symbol{sym},
			Evidence: []Evidence{
				{Kind: "large_parameter_list", Message: "Parameter count is at least 5.", Location: sym.Location, Metrics: map[string]float64{"parameters": float64(n)}},
				advisoryNoOperationEvidence("Introducing a parameter object requires user-chosen type and field names."),
			},
			Metrics: map[string]float64{"parameters": float64(n)},
		})
	}
	return out
}

func suggestBooleanFlags(idx *index) []Proposal {
	var out []Proposal
	for _, sym := range idx.symbols {
		if sym.Kind != SymbolFunction && sym.Kind != SymbolMethod {
			continue
		}
		if !hasBoolParam(sym.Signature) {
			continue
		}
		out = append(out, Proposal{
			Kind:       RefactorReplaceFlagArgument,
			Title:      "Replace boolean flag parameter",
			Summary:    fmt.Sprintf("%q accepts a boolean parameter that may hide behavior choices.", sym.QualifiedName),
			Confidence: MediumConfidence,
			Risk:       RiskMedium,
			Targets:    []Symbol{sym},
			Evidence: []Evidence{
				{Kind: "boolean_flag_parameter", Message: "Function signature contains a bool parameter.", Location: sym.Location},
				advisoryNoOperationEvidence("Replacing a boolean flag requires semantic intent for the alternative API."),
			},
		})
	}
	return out
}

func suggestHighFanIn(idx *index) []Proposal {
	var out []Proposal
	for _, metric := range computeMetrics(idx) {
		if metric.DirectFanIn < 3 && metric.CallFanIn < 5 {
			continue
		}
		out = append(out, Proposal{
			Kind:       RefactorSplitPackage,
			Title:      "Review high fan-in package",
			Summary:    fmt.Sprintf("Unit %q has high inbound pressure.", metric.UnitID),
			Confidence: LowConfidence,
			Risk:       RiskHigh,
			Evidence:   advisoryEvidence(metric.Evidence, "Package split operations require user-selected package boundaries."),
			Metrics: map[string]float64{
				"direct_fan_in": float64(metric.DirectFanIn),
				"call_fan_in":   float64(metric.CallFanIn),
				"score":         metric.PressureScore,
			},
		})
	}
	return out
}

func suggestHighPressureSymbols(idx *index) []Proposal {
	metrics := computeSymbolMetrics(idx)
	var out []Proposal
	for _, metric := range metrics {
		if metric.ReferenceCount < 5 && metric.CallFanIn < 5 {
			continue
		}
		sym := idx.byID[metric.SymbolID]
		if sym.ID == "" || sym.Kind == SymbolField || sym.Kind == SymbolPackage {
			continue
		}
		out = append(out, Proposal{
			Kind:       RefactorSplitFunction,
			Title:      "Review high-pressure symbol",
			Summary:    fmt.Sprintf("%q has high inbound source pressure.", metric.QualifiedName),
			Confidence: LowConfidence,
			Risk:       RiskMedium,
			Targets:    []Symbol{sym},
			Evidence:   advisoryEvidence(metric.Evidence, "High-pressure symbols require user-selected refactoring strategy."),
			Metrics: map[string]float64{
				"references":  float64(metric.ReferenceCount),
				"call_fan_in": float64(metric.CallFanIn),
				"score":       metric.PressureScore,
			},
		})
	}
	return out
}

func sourceHash(ctx context.Context, snapshot Snapshot, sym Symbol) (string, error) {
	src, err := snapshot.ReadFile(ctx, sym.Location.URI)
	if err != nil {
		return "", err
	}
	start, end := sym.Location.Range.Start.Offset, sym.Location.Range.End.Offset
	if start < 0 || end > len(src) || start > end {
		return "", fmt.Errorf("editor: invalid symbol range for %s", sym.QualifiedName)
	}
	return hashBytes(src[start:end]), nil
}

func advisoryNoOperationEvidence(message string) Evidence {
	return Evidence{Kind: "advisory_no_operation", Message: message}
}

func advisoryEvidence(existing []Evidence, message string) []Evidence {
	out := append([]Evidence(nil), existing...)
	return append(out, advisoryNoOperationEvidence(message))
}

func computeSymbolMetrics(idx *index) []SymbolMetrics {
	metrics := map[SymbolID]*SymbolMetrics{}
	for _, sym := range idx.symbols {
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
	for _, occ := range idx.occurrences {
		if occ.Kind == OccurrenceDeclaration || occ.Kind == OccurrenceDoc {
			continue
		}
		m := ensureSymbolMetric(metrics, idx, occ.SymbolID)
		if m == nil {
			continue
		}
		m.ReferenceCount++
	}
	for _, edge := range idx.edges {
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

func ensureSymbolMetric(metrics map[SymbolID]*SymbolMetrics, idx *index, id SymbolID) *SymbolMetrics {
	if id == "" {
		return nil
	}
	if m, ok := metrics[id]; ok {
		return m
	}
	sym, ok := idx.byID[id]
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

func countParams(signature string) int {
	params := parseSignatureParams(signature)
	if params == nil {
		return 0
	}
	count := 0
	for _, field := range params.List {
		if len(field.Names) == 0 {
			count++
		} else {
			count += len(field.Names)
		}
	}
	return count
}

func hasBoolParam(signature string) bool {
	params := parseSignatureParams(signature)
	if params == nil {
		return false
	}
	for _, field := range params.List {
		if ident, ok := field.Type.(*ast.Ident); ok && ident.Name == "bool" {
			return true
		}
	}
	return false
}

func parseSignatureParams(signature string) *ast.FieldList {
	src := strings.TrimSpace(signature)
	if !strings.HasPrefix(src, "func ") {
		return nil
	}
	if !strings.Contains(src, "{") {
		src += " {}"
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "signature.go", "package p\n"+src, 0)
	if err != nil || len(file.Decls) == 0 {
		return nil
	}
	fn, ok := file.Decls[0].(*ast.FuncDecl)
	if !ok || fn.Type == nil {
		return nil
	}
	return fn.Type.Params
}
