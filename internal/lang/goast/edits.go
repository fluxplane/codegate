package goast

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/codewandler/editor/internal/core"
)

type editCompiler struct {
	snapshot Snapshot
}

func (b GoBackend) CompileEdit(ctx context.Context, snapshot Snapshot, op Operation) ([]FileEdit, error) {
	compiler := editCompiler{snapshot: snapshot}
	switch x := op.(type) {
	case RenameSymbol:
		return compiler.compileRenameSymbol(ctx, x)
	case ReplaceSymbol:
		return compiler.compileReplaceSymbol(ctx, x)
	case ReplaceFunction:
		return compiler.compileReplaceFunction(ctx, x)
	case AppendSymbol:
		return compiler.compileAppendSymbol(ctx, x)
	case AppendFunction:
		return compiler.compileAppendFunction(ctx, x)
	case DeleteSymbol:
		return compiler.compileDeleteSymbol(ctx, x)
	case DeleteFunction:
		return compiler.compileDeleteFunction(ctx, x)
	case ReplaceMethod:
		return compiler.compileReplaceMethod(ctx, x)
	case DeleteMethod:
		return compiler.compileDeleteMethod(ctx, x)
	case ReplaceComment:
		return compiler.compileReplaceComment(ctx, x)
	case EnsureGoStructTag:
		return compiler.compileEnsureStructTag(ctx, x)
	case RemoveGoStructTag:
		return compiler.compileRemoveStructTag(ctx, x)
	case EnsureGoImport:
		return compiler.compileEnsureGoImport(ctx, x)
	case RemoveGoImport:
		return compiler.compileRemoveGoImport(ctx, x)
	case RenameGoImport:
		return compiler.compileRenameGoImport(ctx, x)
	case MoveSymbol:
		return compiler.compileMoveSymbol(ctx, x)
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
		return nil, err
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
	if err := c.checkExpectedHash(ctx, sym, op.ExpectedHash); err != nil {
		return nil, err
	}
	return []FileEdit{{Path: sym.Location.URI, Edits: []TextEdit{{
		Path:        sym.Location.URI,
		Range:       sym.Location.Range,
		Replacement: ensureTrailingNewline(op.Source),
	}}}}, nil
}

func (c editCompiler) compileReplaceSymbol(ctx context.Context, op ReplaceSymbol) ([]FileEdit, error) {
	idx, err := buildIndex(ctx, c.snapshot, core.SelectorScope(op.Target))
	if err != nil {
		return nil, err
	}
	sym, err := exactSymbol(idx, op.Target, "symbol")
	if err != nil {
		return nil, err
	}
	if err := supportedDeclarationEdit(sym); err != nil {
		return nil, err
	}
	if err := c.checkExpectedHash(ctx, sym, op.ExpectedHash); err != nil {
		return nil, err
	}
	return []FileEdit{{Path: sym.Location.URI, Edits: []TextEdit{{
		Path:        sym.Location.URI,
		Range:       sym.Location.Range,
		Replacement: ensureTrailingNewline(op.Source),
	}}}}, nil
}

func (c editCompiler) compileAppendFunction(ctx context.Context, op AppendFunction) ([]FileEdit, error) {
	return c.compileAppendTopLevel(ctx, op.Path, op.UnitID, op.Source, "append function")
}

func (c editCompiler) compileAppendSymbol(ctx context.Context, op AppendSymbol) ([]FileEdit, error) {
	return c.compileAppendTopLevel(ctx, op.Path, op.UnitID, op.Source, "append symbol")
}

func (c editCompiler) compileAppendTopLevel(ctx context.Context, opPath, unitID, source, label string) ([]FileEdit, error) {
	p, err := c.selectFile(ctx, opPath, unitID, label)
	if err != nil {
		return nil, err
	}
	src, err := c.snapshot.ReadFile(ctx, p)
	if err != nil {
		return nil, err
	}
	replacement := "\n\n" + strings.TrimSpace(source) + "\n"
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
	if err := c.checkExpectedHash(ctx, sym, op.ExpectedHash); err != nil {
		return nil, err
	}
	src, err := c.snapshot.ReadFile(ctx, sym.Location.URI)
	if err != nil {
		return nil, err
	}
	r := deleteRangeForSymbol(src, sym)
	return []FileEdit{{Path: sym.Location.URI, Edits: []TextEdit{{Path: sym.Location.URI, Range: r, Replacement: ""}}}}, nil
}

func (c editCompiler) compileDeleteFunction(ctx context.Context, op DeleteFunction) ([]FileEdit, error) {
	sel := op.Target
	if sel.Kind == "" {
		sel.Kind = SymbolFunction
	}
	if sel.Kind != SymbolFunction {
		return nil, errors.New("editor: DeleteFunction requires a function target")
	}
	return c.compileDeleteSymbol(ctx, DeleteSymbol{Target: sel, ExpectedHash: op.ExpectedHash})
}

func (c editCompiler) compileReplaceMethod(ctx context.Context, op ReplaceMethod) ([]FileEdit, error) {
	sel := op.Target
	if sel.Kind == "" {
		sel.Kind = SymbolMethod
	}
	if sel.Kind != SymbolMethod {
		return nil, errors.New("editor: ReplaceMethod requires a method target")
	}
	return c.compileReplaceFunction(ctx, ReplaceFunction{Target: sel, Source: op.Source, ExpectedHash: op.ExpectedHash})
}

func (c editCompiler) compileDeleteMethod(ctx context.Context, op DeleteMethod) ([]FileEdit, error) {
	sel := op.Target
	if sel.Kind == "" {
		sel.Kind = SymbolMethod
	}
	if sel.Kind != SymbolMethod {
		return nil, errors.New("editor: DeleteMethod requires a method target")
	}
	return c.compileDeleteSymbol(ctx, DeleteSymbol{Target: sel, ExpectedHash: op.ExpectedHash})
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
	if err := c.checkExpectedHash(ctx, sym, op.ExpectedHash); err != nil {
		return nil, err
	}
	src, err := c.snapshot.ReadFile(ctx, sym.Location.URI)
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

func (c editCompiler) compileRenameSymbol(ctx context.Context, op RenameSymbol) ([]FileEdit, error) {
	if !isValidIdentifier(op.NewName) {
		return nil, fmt.Errorf("editor: invalid Go identifier %q", op.NewName)
	}
	idx, err := buildIndex(ctx, c.snapshot, core.SelectorScope(op.Target))
	if err != nil {
		return nil, err
	}
	matches := core.FilterSymbols(idx.symbols, op.Target)
	if len(matches) != 1 {
		return nil, exactMatchErr("symbol", len(matches))
	}
	sym := matches[0]
	if err := supportedRenameSymbol(sym); err != nil {
		return nil, err
	}
	if sym.Name == op.NewName {
		return nil, nil
	}
	if err := c.checkExpectedHash(ctx, sym, op.ExpectedHash); err != nil {
		return nil, err
	}
	editsByPath := map[string][]TextEdit{}
	seen := map[string]bool{}
	add := func(loc Location) {
		if loc.Range.Start.Offset < 0 || loc.Range.End.Offset < loc.Range.Start.Offset {
			return
		}
		key := loc.URI + ":" + fmt.Sprint(loc.Range.Start.Offset) + ":" + fmt.Sprint(loc.Range.End.Offset)
		if seen[key] {
			return
		}
		seen[key] = true
		editsByPath[loc.URI] = append(editsByPath[loc.URI], TextEdit{Path: loc.URI, Range: loc.Range, Replacement: op.NewName})
	}
	add(Location{URI: sym.Location.URI, Range: sym.SelectionRange})
	for _, occ := range idx.occurrences {
		if occ.SymbolID == sym.ID {
			if occ.Location.Range.End.Offset-occ.Location.Range.Start.Offset != len(sym.Name) {
				continue
			}
			add(occ.Location)
		}
	}
	if len(editsByPath) == 0 {
		return nil, errors.New("editor: rename produced no edits")
	}
	paths := make([]string, 0, len(editsByPath))
	for p := range editsByPath {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	out := make([]FileEdit, 0, len(paths))
	for _, p := range paths {
		out = append(out, FileEdit{Path: p, Edits: editsByPath[p]})
	}
	return out, nil
}

func (c editCompiler) checkExpectedHash(ctx context.Context, sym Symbol, expected string) error {
	if expected == "" {
		return nil
	}
	src, err := c.snapshot.ReadFile(ctx, sym.Location.URI)
	if err != nil {
		return err
	}
	start, end := sym.Location.Range.Start.Offset, sym.Location.Range.End.Offset
	if start < 0 || end > len(src) || start > end {
		return errors.New("editor: invalid symbol range")
	}
	actual := hashBytes(src[start:end])
	if actual != expected {
		return fmt.Errorf("editor: stale source for %s: expected hash %s, got %s", sym.QualifiedName, expected, actual)
	}
	return nil
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func supportedRenameSymbol(sym Symbol) error {
	switch sym.Kind {
	case SymbolFunction, SymbolMethod, SymbolType, SymbolStruct, SymbolInterface, SymbolConst, SymbolVar:
		return nil
	case SymbolField:
		return errors.New("editor: rename of Go struct fields is not supported")
	case SymbolPackage:
		return errors.New("editor: rename of Go packages is not supported")
	default:
		return fmt.Errorf("editor: rename of Go %s symbols is not supported", sym.Kind)
	}
}

func isValidIdentifier(name string) bool {
	if name == "" || name == "_" || token.Lookup(name) != token.IDENT {
		return false
	}
	for i, r := range name {
		if i == 0 {
			if r != '_' && !unicode.IsLetter(r) {
				return false
			}
			continue
		}
		if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
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
	src, err := c.snapshot.ReadFile(ctx, sym.Location.URI)
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
	src, err := c.snapshot.ReadFile(ctx, sym.Location.URI)
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

func (c editCompiler) compileEnsureGoImport(ctx context.Context, op EnsureGoImport) ([]FileEdit, error) {
	if op.ImportPath == "" {
		return nil, errors.New("editor: EnsureGoImport requires ImportPath")
	}
	p, err := c.selectFile(ctx, op.Path, op.UnitID, "ensure import")
	if err != nil {
		return nil, err
	}
	src, err := c.snapshot.ReadFile(ctx, p)
	if err != nil {
		return nil, err
	}
	pf, err := parseOne(p, src)
	if err != nil {
		return nil, err
	}
	if findImportSpec(pf, op.ImportPath, "") != nil {
		return nil, nil
	}
	edit, err := ensureImportEdit(pf, op.ImportPath, op.Alias)
	if err != nil {
		return nil, err
	}
	return []FileEdit{{Path: p, Edits: []TextEdit{edit}}}, nil
}

func (c editCompiler) compileRemoveGoImport(ctx context.Context, op RemoveGoImport) ([]FileEdit, error) {
	if op.ImportPath == "" {
		return nil, errors.New("editor: RemoveGoImport requires ImportPath")
	}
	p, err := c.selectFile(ctx, op.Path, op.UnitID, "remove import")
	if err != nil {
		return nil, err
	}
	src, err := c.snapshot.ReadFile(ctx, p)
	if err != nil {
		return nil, err
	}
	pf, err := parseOne(p, src)
	if err != nil {
		return nil, err
	}
	imp := findImportSpec(pf, op.ImportPath, op.Alias)
	if imp == nil {
		return nil, nil
	}
	r := removeImportRange(pf, imp)
	return []FileEdit{{Path: p, Edits: []TextEdit{{Path: p, Range: r, Replacement: ""}}}}, nil
}

func (c editCompiler) compileRenameGoImport(ctx context.Context, op RenameGoImport) ([]FileEdit, error) {
	if op.ImportPath == "" {
		return nil, errors.New("editor: RenameGoImport requires ImportPath")
	}
	p, err := c.selectFile(ctx, op.Path, op.UnitID, "rename import")
	if err != nil {
		return nil, err
	}
	src, err := c.snapshot.ReadFile(ctx, p)
	if err != nil {
		return nil, err
	}
	pf, err := parseOne(p, src)
	if err != nil {
		return nil, err
	}
	imp := findImportSpec(pf, op.ImportPath, "")
	if imp == nil {
		return nil, fmt.Errorf("editor: import %q not found", op.ImportPath)
	}
	r := rangeOf(pf.fset, imp.Pos(), imp.End())
	return []FileEdit{{Path: p, Edits: []TextEdit{{Path: p, Range: r, Replacement: formatImportSpec(op.ImportPath, op.Alias)}}}}, nil
}

func (c editCompiler) compileMoveSymbol(ctx context.Context, op MoveSymbol) ([]FileEdit, error) {
	if op.ToPath == "" {
		return nil, errors.New("editor: MoveSymbol requires ToPath")
	}
	toPath := core.CleanPath(op.ToPath)
	idx, err := buildIndex(ctx, c.snapshot, core.SelectorScope(op.Target))
	if err != nil {
		return nil, err
	}
	sym, err := exactSymbol(idx, op.Target, "symbol")
	if err != nil {
		return nil, err
	}
	if err := supportedDeclarationEdit(sym); err != nil {
		return nil, err
	}
	if sym.Location.URI == toPath {
		return nil, errors.New("editor: MoveSymbol target file must differ from source file")
	}
	if err := c.checkExpectedHash(ctx, sym, op.ExpectedHash); err != nil {
		return nil, err
	}
	src, err := c.snapshot.ReadFile(ctx, sym.Location.URI)
	if err != nil {
		return nil, err
	}
	target, err := c.snapshot.ReadFile(ctx, toPath)
	if err != nil {
		return nil, err
	}
	sourceText := moveDeclarationSource(sym, strings.TrimSpace(string(src[sym.Location.Range.Start.Offset:sym.Location.Range.End.Offset])))
	deleteRange := deleteRangeForSymbol(src, sym)
	appendRange := Range{Start: Position{Offset: len(target)}, End: Position{Offset: len(target)}}
	replacement := "\n\n" + sourceText + "\n"
	if len(target) == 0 || target[len(target)-1] != '\n' {
		replacement = "\n" + replacement
	}
	return []FileEdit{
		{Path: sym.Location.URI, Edits: []TextEdit{{Path: sym.Location.URI, Range: deleteRange, Replacement: ""}}},
		{Path: toPath, Edits: []TextEdit{{Path: toPath, Range: appendRange, Replacement: replacement}}},
	}, nil
}

func moveDeclarationSource(sym Symbol, source string) string {
	switch sym.Kind {
	case SymbolConst:
		if !strings.HasPrefix(source, "const ") {
			return "const " + source
		}
	case SymbolVar:
		if !strings.HasPrefix(source, "var ") {
			return "var " + source
		}
	case SymbolType, SymbolStruct, SymbolInterface:
		if !strings.HasPrefix(source, "type ") {
			return "type " + source
		}
	}
	return source
}

func exactMatchErr(kind string, n int) error {
	if n == 0 {
		return fmt.Errorf("editor: %s not found", kind)
	}
	return fmt.Errorf("editor: selector is ambiguous: %d %ss match", n, kind)
}

func exactSymbol(idx *index, sel SymbolSelector, kind string) (Symbol, error) {
	matches := core.FilterSymbols(idx.symbols, sel)
	if len(matches) != 1 {
		return Symbol{}, exactMatchErr(kind, len(matches))
	}
	return matches[0], nil
}

func supportedDeclarationEdit(sym Symbol) error {
	switch sym.Kind {
	case SymbolFunction, SymbolMethod, SymbolType, SymbolStruct, SymbolInterface, SymbolConst, SymbolVar:
		return nil
	case SymbolField:
		return errors.New("editor: operation does not support Go struct fields")
	case SymbolPackage:
		return errors.New("editor: operation does not support Go packages")
	default:
		return fmt.Errorf("editor: operation does not support Go %s symbols", sym.Kind)
	}
}

func (c editCompiler) selectFile(ctx context.Context, filePath, unitID, label string) (string, error) {
	if filePath != "" {
		return core.CleanPath(filePath), nil
	}
	if unitID == "" {
		return "", fmt.Errorf("editor: %s requires Path or UnitID", label)
	}
	idx, err := buildIndex(ctx, c.snapshot, Scope{UnitID: unitID, Language: Go})
	if err != nil {
		return "", err
	}
	files := append([]string(nil), idx.unitFiles[unitID]...)
	if len(files) == 0 {
		return "", fmt.Errorf("editor: %s requires Path or valid UnitID", label)
	}
	sort.Strings(files)
	return files[0], nil
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

func deleteRangeForSymbol(src []byte, sym Symbol) Range {
	return expandLineRange(src, sym.Location.Range)
}

func parseOne(p string, src []byte) (parsedFile, error) {
	fset := token.NewFileSet()
	af, err := parser.ParseFile(fset, p, src, parser.ParseComments)
	if err != nil {
		return parsedFile{}, err
	}
	return parsedFile{path: p, src: src, fset: fset, file: af, unit: unitID(p, af.Name.Name)}, nil
}

func ensureImportEdit(pf parsedFile, importPath, alias string) (TextEdit, error) {
	if err := validateImportAlias(alias); err != nil {
		return TextEdit{}, err
	}
	spec := formatImportSpec(importPath, alias)
	decls := importDecls(pf)
	if len(decls) == 0 {
		offset := pf.fset.Position(pf.file.Name.End()).Offset
		return TextEdit{
			Path:        pf.path,
			Range:       Range{Start: Position{Offset: offset}, End: Position{Offset: offset}},
			Replacement: "\n\nimport " + spec,
		}, nil
	}
	gen := decls[len(decls)-1]
	if gen.Lparen.IsValid() {
		offset := pf.fset.Position(gen.Rparen).Offset
		return TextEdit{
			Path:        pf.path,
			Range:       Range{Start: Position{Offset: offset}, End: Position{Offset: offset}},
			Replacement: "\n\t" + spec,
		}, nil
	}
	if len(gen.Specs) != 1 {
		return TextEdit{}, errors.New("editor: malformed import declaration")
	}
	existing := strings.TrimSpace(sourceSlice(pf.src, pf.fset, gen.Specs[0].Pos(), gen.Specs[0].End()))
	replacement := "import (\n\t" + existing + "\n\t" + spec + "\n)"
	return TextEdit{
		Path:        pf.path,
		Range:       rangeOf(pf.fset, gen.Pos(), gen.End()),
		Replacement: replacement,
	}, nil
}

func findImportSpec(pf parsedFile, importPath, alias string) *ast.ImportSpec {
	for _, imp := range pf.file.Imports {
		pathValue, err := strconv.Unquote(imp.Path.Value)
		if err != nil || pathValue != importPath {
			continue
		}
		if alias != "" && importAlias(imp) != alias {
			continue
		}
		return imp
	}
	return nil
}

func removeImportRange(pf parsedFile, imp *ast.ImportSpec) Range {
	gen := importDeclForSpec(pf, imp)
	if gen != nil && (gen.Lparen == token.NoPos || len(gen.Specs) == 1) {
		return expandLineRange(pf.src, rangeOf(pf.fset, gen.Pos(), gen.End()))
	}
	return expandLineRange(pf.src, rangeOf(pf.fset, imp.Pos(), imp.End()))
}

func importDecls(pf parsedFile) []*ast.GenDecl {
	var out []*ast.GenDecl
	for _, decl := range pf.file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if ok && gen.Tok == token.IMPORT {
			out = append(out, gen)
		}
	}
	return out
}

func importDeclForSpec(pf parsedFile, target *ast.ImportSpec) *ast.GenDecl {
	for _, gen := range importDecls(pf) {
		for _, spec := range gen.Specs {
			if spec == target {
				return gen
			}
		}
	}
	return nil
}

func importAlias(imp *ast.ImportSpec) string {
	if imp.Name == nil {
		return ""
	}
	return imp.Name.Name
}

func formatImportSpec(importPath, alias string) string {
	quoted := strconv.Quote(importPath)
	if alias == "" {
		return quoted
	}
	return alias + " " + quoted
}

func validateImportAlias(alias string) error {
	if alias == "" || alias == "." || alias == "_" {
		return nil
	}
	if !isValidIdentifier(alias) {
		return fmt.Errorf("editor: invalid Go import alias %q", alias)
	}
	return nil
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
