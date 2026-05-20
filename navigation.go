package editor

import (
	"fmt"
	"path"
	"strings"

	"github.com/codewandler/editor/internal/core"
)

func (e *Editor) resolvePositionTarget(idx *core.Index, filePath string, offset int, fallbackEnclosing bool) (NavigationTarget, []Symbol, []Diagnostic) {
	filePath = core.CleanPath(filePath)
	target := NavigationTarget{Location: Location{URI: filePath, Range: Range{Start: Position{Offset: offset}, End: Position{Offset: offset}}}}

	for _, occ := range sortedOccurrencesAt(idx.Occurrences, filePath, offset) {
		target.Name = occ.Name
		target.Text = occ.Name
		target.NodeKind = string(occ.Kind)
		target.Location = occ.Location
		if sym, ok := idx.ByID[occ.SymbolID]; ok {
			target.PackageID = sym.UnitID
			if enclosing := enclosingSymbol(idx.Symbols, filePath, offset, sym.ID); enclosing.ID != "" {
				target.EnclosingSymbol = &enclosing
			}
			return target, []Symbol{sym}, nil
		}
	}

	for _, sym := range sortedSymbolsAt(idx.Symbols, filePath, offset, true) {
		target.Name = sym.Name
		target.Text = sym.Name
		target.NodeKind = string(sym.Kind)
		target.PackageID = sym.UnitID
		target.Location = Location{URI: sym.Location.URI, Range: sym.SelectionRange}
		if enclosing := enclosingSymbol(idx.Symbols, filePath, offset, sym.ID); enclosing.ID != "" {
			target.EnclosingSymbol = &enclosing
		}
		return target, []Symbol{sym}, nil
	}

	if fallbackEnclosing {
		if enclosing := enclosingSymbol(idx.Symbols, filePath, offset, ""); enclosing.ID != "" {
			target.Name = enclosing.Name
			target.Text = enclosing.Name
			target.NodeKind = "enclosing_symbol"
			target.PackageID = enclosing.UnitID
			target.Location = enclosing.Location
			target.EnclosingSymbol = &enclosing
			return target, []Symbol{enclosing}, []Diagnostic{{
				Location: target.Location,
				Severity: "info",
				Message:  "no identifier definition resolved; returned enclosing declaration",
			}}
		}
	}

	return target, nil, []Diagnostic{{
		Location: target.Location,
		Severity: "warning",
		Message:  fmt.Sprintf("no AST-level definition found at offset %d", offset),
	}}
}

func sortedOccurrencesAt(occurrences []Occurrence, filePath string, offset int) []Occurrence {
	var out []Occurrence
	for _, occ := range occurrences {
		if core.CleanPath(occ.Location.URI) != filePath {
			continue
		}
		if core.ContainsOffset(occ.Location.Range, offset) {
			out = append(out, occ)
		}
	}
	core.SortOccurrences(out)
	return out
}

func sortedSymbolsAt(symbols []Symbol, filePath string, offset int, selectionOnly bool) []Symbol {
	var out []Symbol
	for _, sym := range symbols {
		if core.CleanPath(sym.Location.URI) != filePath {
			continue
		}
		r := sym.Location.Range
		if selectionOnly {
			r = sym.SelectionRange
		}
		if core.ContainsOffset(r, offset) {
			out = append(out, sym)
		}
	}
	core.SortSymbols(out)
	return out
}

func enclosingSymbol(symbols []Symbol, filePath string, offset int, exclude SymbolID) Symbol {
	var candidates []Symbol
	for _, sym := range symbols {
		if sym.ID == exclude || core.CleanPath(sym.Location.URI) != filePath {
			continue
		}
		if sym.Kind != SymbolFunction && sym.Kind != SymbolMethod && sym.Kind != SymbolStruct && sym.Kind != SymbolInterface && sym.Kind != SymbolType {
			continue
		}
		if core.ContainsOffset(sym.Location.Range, offset) {
			candidates = append(candidates, sym)
		}
	}
	if len(candidates) == 0 {
		return Symbol{}
	}
	core.SortSymbols(candidates)
	best := candidates[0]
	bestSize := rangeSize(best.Location.Range)
	for _, candidate := range candidates[1:] {
		if size := rangeSize(candidate.Location.Range); size < bestSize {
			best = candidate
			bestSize = size
		}
	}
	return best
}

func rangeSize(r Range) int {
	return r.End.Offset - r.Start.Offset
}

func packagePath(packageID string) string {
	if i := strings.IndexByte(packageID, '#'); i >= 0 {
		return packageID[:i]
	}
	return packageID
}

func navigationScopePath(filePath string) string {
	filePath = core.CleanPath(filePath)
	if strings.HasSuffix(filePath, ".go") {
		dir := path.Dir(filePath)
		if dir == "." {
			return ""
		}
		return dir
	}
	return filePath
}
