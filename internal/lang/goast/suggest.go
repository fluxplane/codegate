package goast

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

func (b GoBackend) Suggest(ctx context.Context, snapshot Snapshot, scope Scope) ([]Proposal, error) {
	idx, err := buildIndex(ctx, snapshot, scope)
	if err != nil {
		return nil, err
	}
	var proposals []Proposal
	proposals = append(proposals, suggestUnusedPrivate(idx)...)
	proposals = append(proposals, suggestLargeFunctions(idx)...)
	proposals = append(proposals, suggestLargeParameterLists(idx)...)
	proposals = append(proposals, suggestBooleanFlags(idx)...)
	proposals = append(proposals, suggestHighFanIn(idx)...)
	for i := range proposals {
		proposals[i].ID = fmt.Sprintf("prop_%03d", i+1)
	}
	return proposals, nil
}

func suggestUnusedPrivate(idx *index) []Proposal {
	used := map[SymbolID]bool{}
	for _, occ := range idx.occurrences {
		if occ.Kind == OccurrenceDeclaration {
			continue
		}
		used[occ.SymbolID] = true
	}
	var out []Proposal
	for _, sym := range idx.symbols {
		if sym.Kind == SymbolField || sym.Kind == SymbolMethod || sym.Kind == SymbolPackage || isExported(sym.Name) || used[sym.ID] {
			continue
		}
		if strings.HasPrefix(sym.Name, "_") {
			continue
		}
		out = append(out, Proposal{
			Kind:       RefactorDeleteSymbol,
			Title:      "Delete unused private symbol",
			Summary:    fmt.Sprintf("Private %s %q has no AST-detected references.", sym.Kind, sym.QualifiedName),
			Confidence: MediumConfidence,
			Risk:       RiskLow,
			Targets:    []Symbol{sym},
			Evidence:   []Evidence{{Kind: "unused_private_symbol", Message: "No references were found by the AST backend.", Location: sym.Location}},
			Operations: []Operation{DeleteSymbol{Target: SymbolSelector{ID: sym.ID}}},
			Metrics:    map[string]float64{"references": 0},
		})
	}
	return out
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
			Evidence:   []Evidence{{Kind: "large_function", Message: "Function length exceeds 40 lines.", Location: sym.Location, Metrics: map[string]float64{"lines": float64(lines)}}},
			Metrics:    map[string]float64{"lines": float64(lines)},
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
			Evidence:   []Evidence{{Kind: "large_parameter_list", Message: "Parameter count is at least 5.", Location: sym.Location, Metrics: map[string]float64{"parameters": float64(n)}}},
			Metrics:    map[string]float64{"parameters": float64(n)},
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
			Evidence:   []Evidence{{Kind: "boolean_flag_parameter", Message: "Function signature contains a bool parameter.", Location: sym.Location}},
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
			Evidence:   metric.Evidence,
			Metrics: map[string]float64{
				"direct_fan_in": float64(metric.DirectFanIn),
				"call_fan_in":   float64(metric.CallFanIn),
				"score":         metric.PressureScore,
			},
		})
	}
	return out
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
