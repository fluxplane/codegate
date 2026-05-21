package core

import "sort"

func ContainsOffset(r Range, offset int) bool {
	return offset >= r.Start.Offset && offset <= r.End.Offset
}

func SelectorScope(sel SymbolSelector) Scope {
	scope := Scope{Path: sel.Path, UnitID: sel.UnitID, Language: sel.Language}
	if sel.IncludeTests != nil {
		scope.IncludeTests = *sel.IncludeTests
	}
	return scope
}

func FilterSymbols(symbols []Symbol, sel SymbolSelector) []Symbol {
	var out []Symbol
	for _, sym := range symbols {
		if sel.ID != "" && sym.ID != sel.ID {
			continue
		}
		if sel.Language != "" && sym.Language != sel.Language {
			continue
		}
		if sel.Kind != "" && sym.Kind != sel.Kind {
			continue
		}
		if sel.Name != "" && sym.Name != sel.Name {
			continue
		}
		if sel.QualifiedName != "" && sym.QualifiedName != sel.QualifiedName {
			continue
		}
		if sel.Container != "" && sym.ContainerName != sel.Container && string(sym.ContainerID) != sel.Container {
			continue
		}
		if sel.UnitID != "" && sym.UnitID != sel.UnitID {
			continue
		}
		if sel.Path != "" && CleanPath(sym.Location.URI) != CleanPath(sel.Path) {
			continue
		}
		if sel.IncludeTests != nil && !*sel.IncludeTests && HasTestPath(sym.Location.URI) {
			continue
		}
		out = append(out, sym)
	}
	SortSymbols(out)
	return out
}

func SortSymbols(symbols []Symbol) {
	sort.SliceStable(symbols, func(i, j int) bool {
		a, b := symbols[i], symbols[j]
		if a.Location.URI != b.Location.URI {
			return a.Location.URI < b.Location.URI
		}
		if a.Location.Range.Start.Offset != b.Location.Range.Start.Offset {
			return a.Location.Range.Start.Offset < b.Location.Range.Start.Offset
		}
		return a.QualifiedName < b.QualifiedName
	})
}

func SortOccurrences(occ []Occurrence) {
	sort.SliceStable(occ, func(i, j int) bool {
		a, b := occ[i], occ[j]
		if a.Location.URI != b.Location.URI {
			return a.Location.URI < b.Location.URI
		}
		return a.Location.Range.Start.Offset < b.Location.Range.Start.Offset
	})
}

func HasTestPath(p string) bool {
	return len(p) >= 8 && p[len(p)-8:] == "_test.go"
}

func ImportsForPath(imports []ImportEdge, p string) []ImportEdge {
	var out []ImportEdge
	for _, imp := range imports {
		if CleanPath(imp.FromPath) == CleanPath(p) {
			out = append(out, imp)
		}
	}
	return out
}

// LineColumnOffset converts a 1-indexed line/column pair to a byte offset.
//
// Columns are byte columns, matching go/token.Position.Column. This is
// intentional for Go ranges: non-ASCII UTF-8 characters before the target count
// as multiple columns, so Position.Line/Column stays consistent with stored
// token positions and Offset values.
func LineColumnOffset(b []byte, line, column int) int {
	if line <= 1 && column <= 1 {
		return 0
	}
	curLine, curCol := 1, 1
	for i := 0; i < len(b); i++ {
		if curLine == line && curCol == column {
			return i
		}
		if b[i] == '\n' {
			curLine++
			curCol = 1
		} else {
			curCol++
		}
	}
	return len(b)
}

func PositionForOffset(b []byte, offset int) Position {
	if offset < 0 {
		offset = 0
	}
	if offset > len(b) {
		offset = len(b)
	}
	line, column := 1, 1
	for i := 0; i < offset; i++ {
		if b[i] == '\n' {
			line++
			column = 1
			continue
		}
		column++
	}
	return Position{Line: line, Column: column, Offset: offset}
}

func CleanPath(p string) string {
	if p == "" {
		return "."
	}
	out := pathClean(p)
	if out == "" {
		return "."
	}
	return out
}

func pathClean(p string) string {
	p = replaceSlash(p)
	p = cleanSlashPath(p)
	if len(p) >= 2 && p[:2] == "./" {
		p = p[2:]
	}
	return p
}

func replaceSlash(p string) string {
	b := []byte(p)
	for i, c := range b {
		if c == '\\' {
			b[i] = '/'
		}
	}
	return string(b)
}

func cleanSlashPath(p string) string {
	if p == "" {
		return "."
	}
	parts := []string{}
	for _, part := range splitSlash(p) {
		switch part {
		case "", ".":
			continue
		case "..":
			if len(parts) > 0 {
				parts = parts[:len(parts)-1]
			}
		default:
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		return "."
	}
	out := parts[0]
	for _, part := range parts[1:] {
		out += "/" + part
	}
	return out
}

func splitSlash(p string) []string {
	var out []string
	start := 0
	for i, c := range p {
		if c == '/' {
			out = append(out, p[start:i])
			start = i + 1
		}
	}
	out = append(out, p[start:])
	return out
}
