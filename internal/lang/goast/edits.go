package goast

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"sort"
	"strings"

	"github.com/codewandler/editor/internal/core"
)

type editCompiler struct {
	snapshot Snapshot
}

func (b GoBackend) CompileEdit(ctx context.Context, snapshot Snapshot, op Operation) ([]FileEdit, error) {
	compiler := editCompiler{snapshot: snapshot}
	switch x := op.(type) {
	case ReplaceFunction:
		return compiler.compileReplaceFunction(ctx, x)
	case AppendFunction:
		return compiler.compileAppendFunction(ctx, x)
	case DeleteSymbol:
		return compiler.compileDeleteSymbol(ctx, x)
	case ReplaceComment:
		return compiler.compileReplaceComment(ctx, x)
	case EnsureGoStructTag:
		return compiler.compileEnsureStructTag(ctx, x)
	case RemoveGoStructTag:
		return compiler.compileRemoveStructTag(ctx, x)
	default:
		return nil, fmt.Errorf("editor: go backend does not support operation %q", op.Kind())
	}
}

func (b GoBackend) Format(ctx context.Context, path string, src []byte) ([]byte, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if !strings.HasSuffix(path, ".go") {
		return src, nil
	}
	formatted, err := format.Source(src)
	if err != nil {
		return src, nil
	}
	return formatted, nil
}

func (c editCompiler) compileReplaceFunction(ctx context.Context, op ReplaceFunction) ([]FileEdit, error) {
	sel := op.Target
	if sel.Kind == "" {
		sel.Kind = SymbolFunction
	}
	idx, err := buildIndex(ctx, c.snapshot, core.SelectorScope(sel))
	if err != nil {
		return nil, err
	}
	matches := core.FilterSymbols(idx.symbols, sel)
	if len(matches) == 0 && sel.Kind == SymbolFunction {
		sel.Kind = SymbolMethod
		matches = core.FilterSymbols(idx.symbols, sel)
	}
	if len(matches) != 1 {
		return nil, exactMatchErr("function", len(matches))
	}
	sym := matches[0]
	return []FileEdit{{Path: sym.Location.URI, Edits: []TextEdit{{
		Path:        sym.Location.URI,
		Range:       sym.Location.Range,
		Replacement: ensureTrailingNewline(op.Source),
	}}}}, nil
}

func (c editCompiler) compileAppendFunction(ctx context.Context, op AppendFunction) ([]FileEdit, error) {
	p := core.CleanPath(op.Path)
	if op.Path == "" {
		idx, err := buildIndex(ctx, c.snapshot, Scope{UnitID: op.UnitID, Language: Go})
		if err != nil {
			return nil, err
		}
		files := idx.unitFiles[op.UnitID]
		if len(files) == 0 {
			return nil, errors.New("editor: append function requires Path or valid UnitID")
		}
		sort.Strings(files)
		p = files[0]
	}
	src, err := c.snapshot.ReadFile(p)
	if err != nil {
		return nil, err
	}
	replacement := "\n\n" + strings.TrimSpace(op.Source) + "\n"
	if len(src) == 0 || src[len(src)-1] != '\n' {
		replacement = "\n" + replacement
	}
	r := Range{Start: Position{Offset: len(src)}, End: Position{Offset: len(src)}}
	return []FileEdit{{Path: p, Edits: []TextEdit{{Path: p, Range: r, Replacement: replacement}}}}, nil
}

func (c editCompiler) compileDeleteSymbol(ctx context.Context, op DeleteSymbol) ([]FileEdit, error) {
	idx, err := buildIndex(ctx, c.snapshot, core.SelectorScope(op.Target))
	if err != nil {
		return nil, err
	}
	matches := core.FilterSymbols(idx.symbols, op.Target)
	if len(matches) != 1 {
		return nil, exactMatchErr("symbol", len(matches))
	}
	sym := matches[0]
	if sym.Kind == SymbolField {
		return nil, errors.New("editor: DeleteSymbol does not delete fields; use struct tag operations for field metadata")
	}
	src, err := c.snapshot.ReadFile(sym.Location.URI)
	if err != nil {
		return nil, err
	}
	r := expandLineRange(src, sym.Location.Range)
	return []FileEdit{{Path: sym.Location.URI, Edits: []TextEdit{{Path: sym.Location.URI, Range: r, Replacement: ""}}}}, nil
}

func (c editCompiler) compileReplaceComment(ctx context.Context, op ReplaceComment) ([]FileEdit, error) {
	idx, err := buildIndex(ctx, c.snapshot, core.SelectorScope(op.Target))
	if err != nil {
		return nil, err
	}
	matches := core.FilterSymbols(idx.symbols, op.Target)
	if len(matches) != 1 {
		return nil, exactMatchErr("symbol", len(matches))
	}
	sym := matches[0]
	src, err := c.snapshot.ReadFile(sym.Location.URI)
	if err != nil {
		return nil, err
	}
	pf, err := parseOne(sym.Location.URI, src)
	if err != nil {
		return nil, err
	}
	start, end, err := docRangeForSymbol(pf, sym)
	if err != nil {
		return nil, err
	}
	comment := formatDocComment(sym.Name, op.Text)
	if end == start {
		comment += "\n"
	}
	r := Range{Start: Position{Offset: start}, End: Position{Offset: end}}
	return []FileEdit{{Path: sym.Location.URI, Edits: []TextEdit{{Path: sym.Location.URI, Range: r, Replacement: comment}}}}, nil
}

func (c editCompiler) compileEnsureStructTag(ctx context.Context, op EnsureGoStructTag) ([]FileEdit, error) {
	idx, err := buildIndex(ctx, c.snapshot, core.SelectorScope(op.Struct))
	if err != nil {
		return nil, err
	}
	sel := op.Struct
	if sel.Kind == "" {
		sel.Kind = SymbolStruct
	}
	matches := core.FilterSymbols(idx.symbols, sel)
	if len(matches) != 1 {
		return nil, exactMatchErr("struct", len(matches))
	}
	sym := matches[0]
	src, err := c.snapshot.ReadFile(sym.Location.URI)
	if err != nil {
		return nil, err
	}
	pf, err := parseOne(sym.Location.URI, src)
	if err != nil {
		return nil, err
	}
	field, err := findStructField(pf, sym.Name, op.Field)
	if err != nil {
		return nil, err
	}
	tags := map[string]string{}
	if field.Tag != nil {
		tags = parseTagLiteral(field.Tag.Value)
	}
	value := op.Value
	if len(op.Options) > 0 {
		value = value + "," + strings.Join(op.Options, ",")
	}
	tags[op.Key] = value
	newTag := formatTagLiteral(tags)
	start, end := tagEditOffsets(pf, field)
	replacement := newTag
	if field.Tag == nil {
		replacement = " " + newTag
	}
	r := Range{Start: Position{Offset: start}, End: Position{Offset: end}}
	return []FileEdit{{Path: sym.Location.URI, Edits: []TextEdit{{Path: sym.Location.URI, Range: r, Replacement: replacement}}}}, nil
}

func (c editCompiler) compileRemoveStructTag(ctx context.Context, op RemoveGoStructTag) ([]FileEdit, error) {
	idx, err := buildIndex(ctx, c.snapshot, core.SelectorScope(op.Struct))
	if err != nil {
		return nil, err
	}
	sel := op.Struct
	if sel.Kind == "" {
		sel.Kind = SymbolStruct
	}
	matches := core.FilterSymbols(idx.symbols, sel)
	if len(matches) != 1 {
		return nil, exactMatchErr("struct", len(matches))
	}
	sym := matches[0]
	src, err := c.snapshot.ReadFile(sym.Location.URI)
	if err != nil {
		return nil, err
	}
	pf, err := parseOne(sym.Location.URI, src)
	if err != nil {
		return nil, err
	}
	field, err := findStructField(pf, sym.Name, op.Field)
	if err != nil {
		return nil, err
	}
	if field.Tag == nil {
		return nil, nil
	}
	tags := parseTagLiteral(field.Tag.Value)
	delete(tags, op.Key)
	newTag := formatTagLiteral(tags)
	start, end := tagEditOffsets(pf, field)
	replacement := newTag
	if newTag == "" {
		start = pf.fset.Position(field.Tag.Pos()).Offset
		end = pf.fset.Position(field.Tag.End()).Offset
		if start > 0 && src[start-1] == ' ' {
			start--
		}
	}
	r := Range{Start: Position{Offset: start}, End: Position{Offset: end}}
	return []FileEdit{{Path: sym.Location.URI, Edits: []TextEdit{{Path: sym.Location.URI, Range: r, Replacement: replacement}}}}, nil
}

func exactMatchErr(kind string, n int) error {
	if n == 0 {
		return fmt.Errorf("editor: %s not found", kind)
	}
	return fmt.Errorf("editor: selector is ambiguous: %d %ss match", n, kind)
}

func ensureTrailingNewline(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	return s
}

func expandLineRange(src []byte, r Range) Range {
	start, end := r.Start.Offset, r.End.Offset
	for start > 0 && src[start-1] != '\n' {
		start--
	}
	if end < len(src) && src[end] == '\n' {
		end++
	}
	return Range{Start: Position{Offset: start}, End: Position{Offset: end}}
}

func parseOne(p string, src []byte) (parsedFile, error) {
	fset := token.NewFileSet()
	af, err := parser.ParseFile(fset, p, src, parser.ParseComments)
	if err != nil {
		return parsedFile{}, err
	}
	return parsedFile{path: p, src: src, fset: fset, file: af, unit: unitID(p, af.Name.Name)}, nil
}

func docRangeForSymbol(pf parsedFile, sym Symbol) (int, int, error) {
	for _, decl := range pf.file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			qname := d.Name.Name
			if d.Recv != nil && len(d.Recv.List) > 0 {
				qname = receiverName(d.Recv.List[0].Type) + "." + d.Name.Name
			}
			if qname == sym.QualifiedName {
				if d.Doc != nil {
					return pf.fset.Position(d.Doc.Pos()).Offset, pf.fset.Position(d.Doc.End()).Offset, nil
				}
				return pf.fset.Position(d.Pos()).Offset, pf.fset.Position(d.Pos()).Offset, nil
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if ok && ts.Name.Name == sym.Name {
					if ts.Doc != nil {
						return pf.fset.Position(ts.Doc.Pos()).Offset, pf.fset.Position(ts.Doc.End()).Offset, nil
					}
					if d.Doc != nil {
						return pf.fset.Position(d.Doc.Pos()).Offset, pf.fset.Position(d.Doc.End()).Offset, nil
					}
					return pf.fset.Position(d.Pos()).Offset, pf.fset.Position(d.Pos()).Offset, nil
				}
			}
		}
	}
	return 0, 0, errors.New("editor: declaration not found for comment replacement")
}

func formatDocComment(name, text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		text = name
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = "// " + strings.TrimSpace(line)
	}
	return strings.Join(lines, "\n")
}

func findStructField(pf parsedFile, structName, fieldName string) (*ast.Field, error) {
	for _, decl := range pf.file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || ts.Name.Name != structName {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				return nil, errors.New("editor: target is not a struct")
			}
			for _, field := range st.Fields.List {
				for _, name := range field.Names {
					if name.Name == fieldName {
						return field, nil
					}
				}
			}
		}
	}
	return nil, errors.New("editor: struct field not found")
}

func tagEditOffsets(pf parsedFile, field *ast.Field) (int, int) {
	if field.Tag != nil {
		return pf.fset.Position(field.Tag.Pos()).Offset, pf.fset.Position(field.Tag.End()).Offset
	}
	return pf.fset.Position(field.End()).Offset, pf.fset.Position(field.End()).Offset
}
