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
	"unicode/utf8"

	"github.com/fluxplane/codegate/internal/core"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
)

type editCompiler struct {
	snapshot Snapshot
}

func (b GoBackend) CompileEdit(ctx context.Context, snapshot Snapshot, op Operation) ([]FileEdit, error) {
	compiler := editCompiler{snapshot: snapshot}
	var (
		edits []FileEdit
		err   error
	)
	switch x := op.(type) {
	case RenameSymbol:
		edits, err = compiler.compileRenameSymbol(ctx, x)
	case ReplaceSymbol:
		edits, err = compiler.compileReplaceSymbol(ctx, x)
	case ReplaceFunction:
		edits, err = compiler.compileReplaceFunction(ctx, x)
	case AppendSymbol:
		edits, err = compiler.compileAppendSymbol(ctx, x)
	case AppendFunction:
		edits, err = compiler.compileAppendFunction(ctx, x)
	case DeleteSymbol:
		edits, err = compiler.compileDeleteSymbol(ctx, x)
	case DeleteFunction:
		edits, err = compiler.compileDeleteFunction(ctx, x)
	case ReplaceMethod:
		edits, err = compiler.compileReplaceMethod(ctx, x)
	case DeleteMethod:
		edits, err = compiler.compileDeleteMethod(ctx, x)
	case ReplaceComment:
		edits, err = compiler.compileReplaceComment(ctx, x)
	case EnsureGoStructTag:
		edits, err = compiler.compileEnsureStructTag(ctx, x)
	case RemoveGoStructTag:
		edits, err = compiler.compileRemoveStructTag(ctx, x)
	case EnsureGoImport:
		edits, err = compiler.compileEnsureGoImport(ctx, x)
	case RemoveGoImport:
		edits, err = compiler.compileRemoveGoImport(ctx, x)
	case RenameGoImport:
		edits, err = compiler.compileRenameGoImport(ctx, x)
	case RenameGoModulePath:
		edits, err = compiler.compileRenameGoModulePath(ctx, x)
	case MoveSymbol:
		edits, err = compiler.compileMoveSymbol(ctx, x)
	case AddGoParameter:
		edits, err = compiler.compileAddGoParameter(ctx, x)
	case RemoveGoParameter:
		edits, err = compiler.compileRemoveGoParameter(ctx, x)
	case RenameGoParameter:
		edits, err = compiler.compileRenameGoParameter(ctx, x)
	case AddGoStructField:
		edits, err = compiler.compileAddGoStructField(ctx, x)
	case RemoveGoStructField:
		edits, err = compiler.compileRemoveGoStructField(ctx, x)
	case RenameGoStructField:
		edits, err = compiler.compileRenameGoStructField(ctx, x)
	case ChangeGoParameterType:
		edits, err = compiler.compileChangeGoParameterType(ctx, x)
	case ChangeGoResultType:
		edits, err = compiler.compileChangeGoResultType(ctx, x)
	case RenameGoReceiver:
		edits, err = compiler.compileRenameGoReceiver(ctx, x)
	case AddGoInterfaceMethod:
		edits, err = compiler.compileAddGoInterfaceMethod(ctx, x)
	case RemoveGoInterfaceMethod:
		edits, err = compiler.compileRemoveGoInterfaceMethod(ctx, x)
	case ExtractGoFunction:
		edits, err = compiler.compileExtractGoFunction(ctx, x)
	case ExtractGoMethod:
		edits, err = compiler.compileExtractGoMethod(ctx, x)
	default:
		return nil, fmt.Errorf("codegate: go backend does not support operation %q", op.Kind())
	}
	if err != nil {
		return nil, err
	}
	if err := compiler.rejectGeneratedFileEdits(ctx, edits); err != nil {
		return nil, err
	}
	return edits, nil
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

func (c editCompiler) rejectGeneratedFileEdits(ctx context.Context, edits []FileEdit) error {
	seen := map[string]bool{}
	for _, fe := range edits {
		p := core.CleanPath(fe.Path)
		if seen[p] {
			continue
		}
		seen[p] = true
		if !strings.HasSuffix(p, ".go") {
			continue
		}
		src, err := c.snapshot.ReadFile(ctx, p)
		if err != nil {
			return err
		}
		if isGeneratedGoSource(src) {
			return fmt.Errorf("codegate: refusing to refactor generated Go file %s", p)
		}
	}
	return nil
}

func isGeneratedGoSource(src []byte) bool {
	const marker = "DO NOT EDIT"
	limit := len(src)
	if limit > 4096 {
		limit = 4096
	}
	head := string(src[:limit])
	if !strings.Contains(head, marker) {
		return false
	}
	lines := strings.Split(head, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "//"))
		if strings.HasPrefix(line, "Code generated ") && strings.Contains(line, marker) {
			return true
		}
	}
	return false
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
		return nil, errors.New("codegate: DeleteSymbol does not delete fields; use struct tag operations for field metadata")
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
		return nil, errors.New("codegate: DeleteFunction requires a function target")
	}
	return c.compileDeleteSymbol(ctx, DeleteSymbol{Target: sel, ExpectedHash: op.ExpectedHash})
}

func (c editCompiler) compileReplaceMethod(ctx context.Context, op ReplaceMethod) ([]FileEdit, error) {
	sel := op.Target
	if sel.Kind == "" {
		sel.Kind = SymbolMethod
	}
	if sel.Kind != SymbolMethod {
		return nil, errors.New("codegate: ReplaceMethod requires a method target")
	}
	return c.compileReplaceFunction(ctx, ReplaceFunction{Target: sel, Source: op.Source, ExpectedHash: op.ExpectedHash})
}

func (c editCompiler) compileDeleteMethod(ctx context.Context, op DeleteMethod) ([]FileEdit, error) {
	sel := op.Target
	if sel.Kind == "" {
		sel.Kind = SymbolMethod
	}
	if sel.Kind != SymbolMethod {
		return nil, errors.New("codegate: DeleteMethod requires a method target")
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
		return nil, fmt.Errorf("codegate: invalid Go identifier %q", op.NewName)
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
		return nil, errors.New("codegate: rename produced no edits")
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
		return errors.New("codegate: invalid symbol range")
	}
	actual := hashBytes(src[start:end])
	if actual != expected {
		return fmt.Errorf("codegate: stale source for %s: expected hash %s, got %s", sym.QualifiedName, expected, actual)
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
		return errors.New("codegate: rename of Go struct fields is not supported")
	case SymbolPackage:
		return errors.New("codegate: rename of Go packages is not supported")
	default:
		return fmt.Errorf("codegate: rename of Go %s symbols is not supported", sym.Kind)
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
		return nil, errors.New("codegate: EnsureGoImport requires ImportPath")
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
		return nil, errors.New("codegate: RemoveGoImport requires ImportPath")
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
		return nil, errors.New("codegate: RenameGoImport requires ImportPath")
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
		return nil, fmt.Errorf("codegate: import %q not found", op.ImportPath)
	}
	r := rangeOf(pf.fset, imp.Pos(), imp.End())
	return []FileEdit{{Path: p, Edits: []TextEdit{{Path: p, Range: r, Replacement: formatImportSpec(op.ImportPath, op.Alias)}}}}, nil
}

func (c editCompiler) compileRenameGoModulePath(ctx context.Context, op RenameGoModulePath) ([]FileEdit, error) {
	if err := validateGoModulePath("OldPath", op.OldPath); err != nil {
		return nil, err
	}
	if err := validateGoModulePath("NewPath", op.NewPath); err != nil {
		return nil, err
	}
	if op.OldPath == op.NewPath {
		return nil, errors.New("codegate: RenameGoModulePath requires different old and new paths")
	}
	modSrc, err := c.snapshot.ReadFile(ctx, "go.mod")
	if err != nil {
		return nil, fmt.Errorf("codegate: read go.mod: %w", err)
	}
	modEdit, err := renameModuleDirectiveEdit(modSrc, op.OldPath, op.NewPath)
	if err != nil {
		return nil, err
	}
	files, err := c.snapshot.ListFiles(ctx, Scope{Language: Go, IncludeTests: true})
	if err != nil {
		return nil, err
	}
	out := []FileEdit{{Path: "go.mod", Edits: []TextEdit{modEdit}}}
	for _, p := range files {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		p = core.CleanPath(p)
		if !moduleRenameGoFile(p) {
			continue
		}
		src, err := c.snapshot.ReadFile(ctx, p)
		if err != nil {
			return nil, err
		}
		edits, err := renameModuleImportEdits(p, src, op.OldPath, op.NewPath)
		if err != nil {
			return nil, err
		}
		if len(edits) > 0 {
			out = append(out, FileEdit{Path: p, Edits: edits})
		}
	}
	if len(out) == 1 && len(out[0].Edits) == 0 {
		return nil, errors.New("codegate: RenameGoModulePath produced no edits")
	}
	return out, nil
}

func validateGoModulePath(field, path string) error {
	if path == "" {
		return fmt.Errorf("codegate: RenameGoModulePath requires %s", field)
	}
	if strings.TrimSpace(path) != path {
		return fmt.Errorf("codegate: RenameGoModulePath %s must not contain leading or trailing whitespace", field)
	}
	if err := module.CheckPath(path); err == nil {
		return nil
	}
	if err := module.CheckImportPath(path); err != nil {
		return fmt.Errorf("codegate: invalid RenameGoModulePath %s %q: %w", field, path, err)
	}
	return nil
}

func renameModuleDirectiveEdit(src []byte, oldPath, newPath string) (TextEdit, error) {
	f, err := modfile.Parse("go.mod", src, nil)
	if err != nil {
		return TextEdit{}, fmt.Errorf("codegate: parse go.mod: %w", err)
	}
	if f.Module == nil || f.Module.Syntax == nil {
		return TextEdit{}, errors.New("codegate: go.mod has no module directive")
	}
	if f.Module.Mod.Path != oldPath {
		return TextEdit{}, fmt.Errorf("codegate: go.mod module path is %q, not %q", f.Module.Mod.Path, oldPath)
	}
	start := f.Module.Syntax.Start.Byte
	end := f.Module.Syntax.End.Byte
	if start < 0 || end < start || end > len(src) {
		return TextEdit{}, errors.New("codegate: go.mod module directive has invalid source range")
	}
	lineEnd := end
	for lineEnd < len(src) && src[lineEnd] != '\n' && src[lineEnd] != '\r' {
		lineEnd++
	}
	line := string(src[start:lineEnd])
	oldToken := modfile.AutoQuote(oldPath)
	newToken := modfile.AutoQuote(newPath)
	rel := strings.Index(line, oldToken)
	if rel < 0 && oldToken != oldPath {
		rel = strings.Index(line, oldPath)
		oldToken = oldPath
	}
	if rel < 0 {
		return TextEdit{}, fmt.Errorf("codegate: module directive source does not contain %q", oldPath)
	}
	return TextEdit{
		Path:        "go.mod",
		Range:       Range{Start: Position{Offset: start + rel}, End: Position{Offset: start + rel + len(oldToken)}},
		Replacement: newToken,
	}, nil
}

func renameModuleImportEdits(path string, src []byte, oldPath, newPath string) ([]TextEdit, error) {
	pf, err := parseOne(path, src)
	if err != nil {
		return nil, err
	}
	imports := map[*ast.ImportSpec]string{}
	importPathOffsets := map[int]bool{}
	for _, imp := range pf.file.Imports {
		importPath, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return nil, fmt.Errorf("codegate: parse import path in %s: %w", path, err)
		}
		imports[imp] = importPath
		importPathOffsets[pf.fset.Position(imp.Path.Pos()).Offset] = true
	}
	replacements := map[*ast.ImportSpec]string{}
	for imp, importPath := range imports {
		if rewritten, ok := rewriteModuleImportPath(importPath, oldPath, newPath); ok {
			replacements[imp] = rewritten
		}
	}
	for changed, replacement := range replacements {
		for imp, existing := range imports {
			if imp != changed && existing == replacement {
				return nil, fmt.Errorf("codegate: %s would contain duplicate import %q after module rename", path, replacement)
			}
		}
	}
	edits := make([]TextEdit, 0, len(replacements))
	for imp, replacement := range replacements {
		r := rangeOf(pf.fset, imp.Path.Pos(), imp.Path.End())
		edits = append(edits, TextEdit{
			Path:        path,
			Range:       r,
			Replacement: strconv.Quote(replacement),
		})
	}
	ast.Inspect(pf.file, func(node ast.Node) bool {
		lit, ok := node.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		start := pf.fset.Position(lit.Pos()).Offset
		if importPathOffsets[start] {
			return true
		}
		value, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}
		next, ok := rewriteModulePathReferences(value, oldPath, newPath)
		if !ok {
			return true
		}
		edits = append(edits, TextEdit{
			Path:        path,
			Range:       rangeOf(pf.fset, lit.Pos(), lit.End()),
			Replacement: formatRenamedModuleStringLiteral(lit.Value, next),
		})
		return true
	})
	sort.Slice(edits, func(i, j int) bool {
		return edits[i].Range.Start.Offset < edits[j].Range.Start.Offset
	})
	return edits, nil
}

func rewriteModuleImportPath(importPath, oldPath, newPath string) (string, bool) {
	switch {
	case importPath == oldPath:
		return newPath, true
	case strings.HasPrefix(importPath, oldPath+"/"):
		return newPath + strings.TrimPrefix(importPath, oldPath), true
	default:
		return "", false
	}
}

func rewriteModulePathReferences(value, oldPath, newPath string) (string, bool) {
	var b strings.Builder
	changed := false
	cursor := 0
	for {
		rel := strings.Index(value[cursor:], oldPath)
		if rel < 0 {
			break
		}
		start := cursor + rel
		end := start + len(oldPath)
		if !modulePathReferenceStart(value, start) || !modulePathReferenceEnd(value, end) {
			b.WriteString(value[cursor:end])
			cursor = end
			continue
		}
		b.WriteString(value[cursor:start])
		b.WriteString(newPath)
		cursor = end
		changed = true
	}
	if !changed {
		return value, false
	}
	b.WriteString(value[cursor:])
	return b.String(), true
}

func modulePathReferenceStart(value string, start int) bool {
	return start == 0 || !modulePathTokenByte(value[start-1])
}

func modulePathReferenceEnd(value string, end int) bool {
	return end == len(value) || value[end] == '/' || !modulePathTokenByte(value[end])
}

func modulePathTokenByte(b byte) bool {
	return b == '/' || b == '-' || b == '.' || b == '_' || b == '~' ||
		b >= '0' && b <= '9' ||
		b >= 'A' && b <= 'Z' ||
		b >= 'a' && b <= 'z'
}

func formatRenamedModuleStringLiteral(original, value string) string {
	if strings.HasPrefix(original, "`") && !strings.Contains(value, "`") {
		return "`" + value + "`"
	}
	return strconv.Quote(value)
}

func moduleRenameGoFile(path string) bool {
	if !strings.HasSuffix(path, ".go") {
		return false
	}
	if path == "vendor" || strings.HasPrefix(path, "vendor/") || strings.Contains(path, "/vendor/") {
		return false
	}
	if strings.HasPrefix(path, ".git/") || strings.HasPrefix(path, ".cache/") || strings.HasPrefix(path, "tmp/") || strings.HasPrefix(path, ".tmp/") {
		return false
	}
	return true
}

func (c editCompiler) compileMoveSymbol(ctx context.Context, op MoveSymbol) ([]FileEdit, error) {
	if op.ToPath == "" {
		return nil, errors.New("codegate: MoveSymbol requires ToPath")
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
		return nil, errors.New("codegate: MoveSymbol target file must differ from source file")
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
	edits := []FileEdit{
		{Path: sym.Location.URI, Edits: []TextEdit{{Path: sym.Location.URI, Range: deleteRange, Replacement: ""}}},
		{Path: toPath, Edits: []TextEdit{{Path: toPath, Range: appendRange, Replacement: replacement}}},
	}
	if op.ReconcileImports {
		extra, err := c.moveImportEdits(ctx, sym, sourceText, toPath)
		if err != nil {
			return nil, err
		}
		edits = append(edits, extra...)
	}
	return edits, nil
}

func (c editCompiler) compileAddGoParameter(ctx context.Context, op AddGoParameter) ([]FileEdit, error) {
	if !isValidIdentifier(op.Name) {
		return nil, fmt.Errorf("codegate: invalid Go parameter name %q", op.Name)
	}
	if strings.TrimSpace(op.Type) == "" {
		return nil, errors.New("codegate: AddGoParameter requires Type")
	}
	if strings.TrimSpace(op.DefaultValue) == "" {
		return nil, errors.New("codegate: AddGoParameter requires DefaultValue")
	}
	idx, sym, pf, fn, err := c.resolveFunctionDecl(ctx, op.Target)
	if err != nil {
		return nil, err
	}
	if err := c.checkExpectedHash(ctx, sym, op.ExpectedHash); err != nil {
		return nil, err
	}
	if err := validateAddParameterName(fn, op.Name); err != nil {
		return nil, err
	}
	if err := c.ensureFunctionSignatureSafe(ctx, idx, sym, fn, false); err != nil {
		return nil, err
	}
	paramText := op.Name + " " + strings.TrimSpace(op.Type)
	edits := []FileEdit{{Path: pf.path, Edits: []TextEdit{insertListItemEdit(pf.fset, fn.Type.Params.Opening, fn.Type.Params.Closing, paramRanges(pf.fset, fn.Type.Params), op.Position, paramText)}}}
	callEdits, err := c.callArgumentEdits(ctx, idx, sym, op.Position, op.DefaultValue, true)
	if err != nil {
		return nil, err
	}
	return mergeFileEdits(append(edits, callEdits...)), nil
}

func (c editCompiler) compileRemoveGoParameter(ctx context.Context, op RemoveGoParameter) ([]FileEdit, error) {
	if op.Name == "" {
		return nil, errors.New("codegate: RemoveGoParameter requires Name")
	}
	idx, sym, pf, fn, err := c.resolveFunctionDecl(ctx, op.Target)
	if err != nil {
		return nil, err
	}
	if err := c.checkExpectedHash(ctx, sym, op.ExpectedHash); err != nil {
		return nil, err
	}
	if err := c.ensureFunctionSignatureSafe(ctx, idx, sym, fn, false); err != nil {
		return nil, err
	}
	params := namedParamRanges(pf.fset, fn.Type.Params)
	pos := -1
	var target Range
	var groupNames int
	for i, param := range params {
		if param.name == op.Name {
			pos = i
			target = param.r
			groupNames = param.groupNames
			if groupNames > 1 {
				target = param.nameRange
			}
			break
		}
	}
	if pos < 0 {
		return nil, fmt.Errorf("codegate: parameter %q not found", op.Name)
	}
	replacement := ""
	if groupNames > 1 {
		replacement = removeGroupedNameReplacement(pf.src, target)
	}
	edits := []FileEdit{{Path: pf.path, Edits: []TextEdit{{Path: pf.path, Range: removeListItemRange(pf.src, target), Replacement: replacement}}}}
	callEdits, err := c.callArgumentEdits(ctx, idx, sym, pos, "", false)
	if err != nil {
		return nil, err
	}
	return mergeFileEdits(append(edits, callEdits...)), nil
}

func (c editCompiler) compileRenameGoParameter(ctx context.Context, op RenameGoParameter) ([]FileEdit, error) {
	if op.OldName == "" || !isValidIdentifier(op.NewName) {
		return nil, errors.New("codegate: RenameGoParameter requires valid OldName and NewName")
	}
	_, sym, pf, fn, err := c.resolveFunctionDecl(ctx, op.Target)
	if err != nil {
		return nil, err
	}
	if err := c.checkExpectedHash(ctx, sym, op.ExpectedHash); err != nil {
		return nil, err
	}
	if err := validateRenameParameterName(fn, op.OldName, op.NewName); err != nil {
		return nil, err
	}
	var edits []TextEdit
	found := false
	for _, param := range namedParamRanges(pf.fset, fn.Type.Params) {
		if param.name == op.OldName {
			found = true
			edits = append(edits, TextEdit{Path: pf.path, Range: param.nameRange, Replacement: op.NewName})
		}
	}
	if !found {
		return nil, fmt.Errorf("codegate: parameter %q not found", op.OldName)
	}
	if fn.Body != nil {
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if ok && id.Name == op.OldName {
				edits = append(edits, TextEdit{Path: pf.path, Range: rangeOf(pf.fset, id.Pos(), id.End()), Replacement: op.NewName})
			}
			return true
		})
	}
	return []FileEdit{{Path: pf.path, Edits: edits}}, nil
}

func (c editCompiler) compileAddGoStructField(ctx context.Context, op AddGoStructField) ([]FileEdit, error) {
	if !isValidIdentifier(op.Name) {
		return nil, fmt.Errorf("codegate: invalid Go field name %q", op.Name)
	}
	if strings.TrimSpace(op.Type) == "" {
		return nil, errors.New("codegate: AddGoStructField requires Type")
	}
	sym, pf, st, err := c.resolveStructType(ctx, op.Struct)
	if err != nil {
		return nil, err
	}
	if err := c.checkExpectedHash(ctx, sym, op.ExpectedHash); err != nil {
		return nil, err
	}
	for _, field := range st.Fields.List {
		for _, name := range field.Names {
			if name.Name == op.Name {
				return nil, fmt.Errorf("codegate: field %q already exists", op.Name)
			}
		}
	}
	line := op.Name + " " + strings.TrimSpace(op.Type)
	if op.Tag != "" {
		line += " `" + strings.Trim(op.Tag, "`") + "`"
	}
	if op.Comment != "" {
		line = "// " + strings.TrimSpace(op.Comment) + "\n" + line
	}
	fields := fieldRanges(pf.fset, st.Fields)
	edit := insertStructFieldEdit(pf.fset, st.Fields.Opening, st.Fields.Closing, fields, op.Position, line)
	edit.Path = pf.path
	return []FileEdit{{Path: pf.path, Edits: []TextEdit{edit}}}, nil
}

func (c editCompiler) compileRemoveGoStructField(ctx context.Context, op RemoveGoStructField) ([]FileEdit, error) {
	if op.Field == "" {
		return nil, errors.New("codegate: RemoveGoStructField requires Field")
	}
	idx, err := buildIndex(ctx, c.snapshot, core.SelectorScope(op.Struct))
	if err != nil {
		return nil, err
	}
	sym, pf, st, err := c.resolveStructTypeFromIndex(ctx, idx, op.Struct)
	if err != nil {
		return nil, err
	}
	if err := c.checkExpectedHash(ctx, sym, op.ExpectedHash); err != nil {
		return nil, err
	}
	if fieldNameAmbiguous(idx, sym, op.Field) && hasSelectorUse(ctx, c.snapshot, idx, sym.UnitID, op.Field) {
		return nil, fmt.Errorf("codegate: field %q selector ownership is ambiguous", op.Field)
	}
	var fieldSym Symbol
	for _, child := range sym.Children {
		if child.Name == op.Field {
			fieldSym = child
			break
		}
	}
	if fieldSym.ID == "" {
		return nil, fmt.Errorf("codegate: field %q not found", op.Field)
	}
	for _, occ := range idx.occurrences {
		if occ.SymbolID == fieldSym.ID && occ.Kind != OccurrenceDeclaration {
			return nil, fmt.Errorf("codegate: field %q has indexed references", op.Field)
		}
	}
	for _, field := range st.Fields.List {
		for _, name := range field.Names {
			if name.Name == op.Field {
				r := expandLineRange(pf.src, rangeOf(pf.fset, field.Pos(), field.End()))
				replacement := ""
				if len(field.Names) > 1 {
					r = removeListItemRange(pf.src, rangeOf(pf.fset, name.Pos(), name.End()))
					replacement = removeGroupedNameReplacement(pf.src, rangeOf(pf.fset, name.Pos(), name.End()))
				}
				return []FileEdit{{Path: pf.path, Edits: []TextEdit{{Path: pf.path, Range: r, Replacement: replacement}}}}, nil
			}
		}
	}
	return nil, fmt.Errorf("codegate: field %q not found", op.Field)
}

func (c editCompiler) compileRenameGoStructField(ctx context.Context, op RenameGoStructField) ([]FileEdit, error) {
	if op.OldName == "" || !isValidIdentifier(op.NewName) {
		return nil, errors.New("codegate: RenameGoStructField requires valid OldName and NewName")
	}
	idx, err := buildIndex(ctx, c.snapshot, core.SelectorScope(op.Struct))
	if err != nil {
		return nil, err
	}
	sym, pf, st, err := c.resolveStructTypeFromIndex(ctx, idx, op.Struct)
	if err != nil {
		return nil, err
	}
	if err := c.checkExpectedHash(ctx, sym, op.ExpectedHash); err != nil {
		return nil, err
	}
	if op.UpdateSelectors && fieldNameAmbiguous(idx, sym, op.OldName) {
		return nil, fmt.Errorf("codegate: field %q selector ownership is ambiguous", op.OldName)
	}
	if structHasField(st, op.NewName) {
		return nil, fmt.Errorf("codegate: field %q already exists", op.NewName)
	}
	fieldName, err := findStructFieldName(st, op.OldName)
	if err != nil {
		return nil, err
	}
	edits := []FileEdit{{Path: pf.path, Edits: []TextEdit{{Path: pf.path, Range: rangeOf(pf.fset, fieldName.Pos(), fieldName.End()), Replacement: op.NewName}}}}
	usageEdits, err := c.structFieldUsageEdits(ctx, idx, sym, op.OldName, op.NewName, op.UpdateSelectors)
	if err != nil {
		return nil, err
	}
	return mergeFileEdits(append(edits, usageEdits...)), nil
}

func (c editCompiler) compileChangeGoParameterType(ctx context.Context, op ChangeGoParameterType) ([]FileEdit, error) {
	if op.Name == "" || strings.TrimSpace(op.Type) == "" {
		return nil, errors.New("codegate: ChangeGoParameterType requires Name and Type")
	}
	idx, sym, pf, fn, err := c.resolveFunctionDecl(ctx, op.Target)
	if err != nil {
		return nil, err
	}
	if err := c.checkExpectedHash(ctx, sym, op.ExpectedHash); err != nil {
		return nil, err
	}
	if err := c.ensureFunctionSignatureSafe(ctx, idx, sym, fn, true); err != nil {
		return nil, err
	}
	r, err := namedFieldTypeRange(pf.fset, fn.Type.Params, op.Name, "parameter")
	if err != nil {
		return nil, err
	}
	return []FileEdit{{Path: pf.path, Edits: []TextEdit{{Path: pf.path, Range: r, Replacement: strings.TrimSpace(op.Type)}}}}, nil
}

func (c editCompiler) compileChangeGoResultType(ctx context.Context, op ChangeGoResultType) ([]FileEdit, error) {
	if strings.TrimSpace(op.Type) == "" {
		return nil, errors.New("codegate: ChangeGoResultType requires Type")
	}
	idx, sym, pf, fn, err := c.resolveFunctionDecl(ctx, op.Target)
	if err != nil {
		return nil, err
	}
	if err := c.checkExpectedHash(ctx, sym, op.ExpectedHash); err != nil {
		return nil, err
	}
	if err := c.ensureFunctionSignatureSafe(ctx, idx, sym, fn, true); err != nil {
		return nil, err
	}
	var r Range
	if op.Name != "" {
		r, err = namedFieldTypeRange(pf.fset, fn.Type.Results, op.Name, "result")
	} else {
		r, err = resultTypeRange(pf.fset, fn.Type.Results, op.Position)
	}
	if err != nil {
		return nil, err
	}
	return []FileEdit{{Path: pf.path, Edits: []TextEdit{{Path: pf.path, Range: r, Replacement: strings.TrimSpace(op.Type)}}}}, nil
}

func (c editCompiler) compileRenameGoReceiver(ctx context.Context, op RenameGoReceiver) ([]FileEdit, error) {
	if !isValidIdentifier(op.NewName) {
		return nil, fmt.Errorf("codegate: invalid Go receiver name %q", op.NewName)
	}
	_, sym, pf, fn, err := c.resolveFunctionDecl(ctx, op.Target)
	if err != nil {
		return nil, err
	}
	if sym.Kind != SymbolMethod || fn.Recv == nil || len(fn.Recv.List) == 0 {
		return nil, errors.New("codegate: RenameGoReceiver requires a method target")
	}
	if err := c.checkExpectedHash(ctx, sym, op.ExpectedHash); err != nil {
		return nil, err
	}
	recv := fn.Recv.List[0]
	if len(recv.Names) != 1 {
		return nil, errors.New("codegate: method receiver must have exactly one name")
	}
	oldName := recv.Names[0].Name
	if oldName == op.NewName {
		return nil, nil
	}
	edits := []TextEdit{{Path: pf.path, Range: rangeOf(pf.fset, recv.Names[0].Pos(), recv.Names[0].End()), Replacement: op.NewName}}
	if fn.Body != nil {
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if ok && id.Name == oldName {
				edits = append(edits, TextEdit{Path: pf.path, Range: rangeOf(pf.fset, id.Pos(), id.End()), Replacement: op.NewName})
			}
			return true
		})
	}
	return []FileEdit{{Path: pf.path, Edits: edits}}, nil
}

func (c editCompiler) compileAddGoInterfaceMethod(ctx context.Context, op AddGoInterfaceMethod) ([]FileEdit, error) {
	method := strings.TrimSpace(op.Method)
	if method == "" {
		return nil, errors.New("codegate: AddGoInterfaceMethod requires Method")
	}
	sym, pf, iface, err := c.resolveInterfaceType(ctx, op.Interface)
	if err != nil {
		return nil, err
	}
	if err := c.checkExpectedHash(ctx, sym, op.ExpectedHash); err != nil {
		return nil, err
	}
	name, err := interfaceMethodName(method)
	if err != nil {
		return nil, err
	}
	if interfaceHasMethod(iface, name) {
		return nil, fmt.Errorf("codegate: interface method %q already exists", name)
	}
	edit := insertStructFieldEdit(pf.fset, iface.Methods.Opening, iface.Methods.Closing, fieldRanges(pf.fset, iface.Methods), op.Position, method)
	edit.Path = pf.path
	return []FileEdit{{Path: pf.path, Edits: []TextEdit{edit}}}, nil
}

func (c editCompiler) compileRemoveGoInterfaceMethod(ctx context.Context, op RemoveGoInterfaceMethod) ([]FileEdit, error) {
	if op.Method == "" {
		return nil, errors.New("codegate: RemoveGoInterfaceMethod requires Method")
	}
	idx, err := buildIndex(ctx, c.snapshot, core.SelectorScope(op.Interface))
	if err != nil {
		return nil, err
	}
	sym, pf, iface, err := c.resolveInterfaceTypeFromIndex(ctx, idx, op.Interface)
	if err != nil {
		return nil, err
	}
	if err := c.checkExpectedHash(ctx, sym, op.ExpectedHash); err != nil {
		return nil, err
	}
	var methodSym Symbol
	for _, child := range sym.Children {
		if child.Name == op.Method {
			methodSym = child
			break
		}
	}
	if methodSym.ID == "" {
		return nil, fmt.Errorf("codegate: interface method %q not found", op.Method)
	}
	for _, occ := range idx.occurrences {
		if occ.SymbolID == methodSym.ID && occ.Kind != OccurrenceDeclaration {
			return nil, fmt.Errorf("codegate: interface method %q has indexed references", op.Method)
		}
	}
	for _, method := range iface.Methods.List {
		for _, name := range method.Names {
			if name.Name == op.Method {
				r := expandLineRange(pf.src, rangeOf(pf.fset, method.Pos(), method.End()))
				return []FileEdit{{Path: pf.path, Edits: []TextEdit{{Path: pf.path, Range: r, Replacement: ""}}}}, nil
			}
		}
	}
	return nil, fmt.Errorf("codegate: interface method %q not found", op.Method)
}

func (c editCompiler) compileExtractGoFunction(ctx context.Context, op ExtractGoFunction) ([]FileEdit, error) {
	return c.compileExtract(ctx, op.Path, op.Range, "", op.Name, op.Params, op.Results, op.InsertAfter, op.ReplaceWithCall)
}

func (c editCompiler) compileExtractGoMethod(ctx context.Context, op ExtractGoMethod) ([]FileEdit, error) {
	return c.compileExtract(ctx, op.Path, op.Range, op.Receiver, op.Name, op.Params, op.Results, op.InsertAfter, op.ReplaceWithCall)
}

func (c editCompiler) compileExtract(ctx context.Context, p string, r Range, receiver, name, params, results string, insertAfter SymbolSelector, replaceWithCall string) ([]FileEdit, error) {
	p = core.CleanPath(p)
	if p == "." || name == "" || !isValidIdentifier(name) {
		return nil, errors.New("codegate: extract requires Path and valid Name")
	}
	src, err := c.snapshot.ReadFile(ctx, p)
	if err != nil {
		return nil, err
	}
	if r.Start.Offset < 0 || r.End.Offset <= r.Start.Offset || r.End.Offset > len(src) {
		return nil, errors.New("codegate: invalid extraction range")
	}
	idx, err := buildIndex(ctx, c.snapshot, Scope{Path: p, Language: Go})
	if err != nil {
		return nil, err
	}
	for _, sym := range idx.symbols {
		if sym.Location.URI == p && sym.Name == name {
			return nil, fmt.Errorf("codegate: symbol %q already exists", name)
		}
	}
	insertOffset := len(src)
	if insertAfter.Name != "" || insertAfter.ID != "" || insertAfter.QualifiedName != "" {
		insertAfter.Path = p
		sym, err := exactSymbol(idx, insertAfter, "insert-after symbol")
		if err != nil {
			return nil, err
		}
		insertOffset = sym.Location.Range.End.Offset
	} else if enclosing := enclosingFunction(idx, p, r.Start.Offset, r.End.Offset); enclosing.ID != "" {
		insertOffset = enclosing.Location.Range.End.Offset
	}
	body := strings.TrimSpace(string(src[r.Start.Offset:r.End.Offset]))
	if body == "" {
		return nil, errors.New("codegate: extraction body is empty")
	}
	decl := formatExtractedDecl(receiver, name, params, results, body)
	edits := []TextEdit{{Path: p, Range: Range{Start: Position{Offset: insertOffset}, End: Position{Offset: insertOffset}}, Replacement: "\n\n" + decl + "\n"}}
	if replaceWithCall != "" {
		edits = append(edits, TextEdit{Path: p, Range: r, Replacement: replaceWithCall})
	}
	return []FileEdit{{Path: p, Edits: edits}}, nil
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
		return fmt.Errorf("codegate: %s not found", kind)
	}
	return fmt.Errorf("codegate: selector is ambiguous: %d %ss match", n, kind)
}

func exactSymbol(idx *index, sel SymbolSelector, kind string) (Symbol, error) {
	matches := core.FilterSymbols(idx.symbols, sel)
	if len(matches) != 1 {
		return Symbol{}, exactMatchErr(kind, len(matches))
	}
	return matches[0], nil
}

func mergeFileEdits(fileEdits []FileEdit) []FileEdit {
	byPath := map[string][]TextEdit{}
	for _, fe := range fileEdits {
		byPath[fe.Path] = append(byPath[fe.Path], fe.Edits...)
	}
	paths := make([]string, 0, len(byPath))
	for p := range byPath {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	out := make([]FileEdit, 0, len(paths))
	for _, p := range paths {
		out = append(out, FileEdit{Path: p, Edits: byPath[p]})
	}
	return out
}

func supportedDeclarationEdit(sym Symbol) error {
	switch sym.Kind {
	case SymbolFunction, SymbolMethod, SymbolType, SymbolStruct, SymbolInterface, SymbolConst, SymbolVar:
		return nil
	case SymbolField:
		return errors.New("codegate: operation does not support Go struct fields")
	case SymbolPackage:
		return errors.New("codegate: operation does not support Go packages")
	default:
		return fmt.Errorf("codegate: operation does not support Go %s symbols", sym.Kind)
	}
}

func (c editCompiler) selectFile(ctx context.Context, filePath, unitID, label string) (string, error) {
	if filePath != "" {
		return core.CleanPath(filePath), nil
	}
	if unitID == "" {
		return "", fmt.Errorf("codegate: %s requires Path or UnitID", label)
	}
	idx, err := buildIndex(ctx, c.snapshot, Scope{UnitID: unitID, Language: Go})
	if err != nil {
		return "", err
	}
	files := append([]string(nil), idx.unitFiles[unitID]...)
	if len(files) == 0 {
		return "", fmt.Errorf("codegate: %s requires Path or valid UnitID", label)
	}
	sort.Strings(files)
	return files[0], nil
}

type rangeItem struct {
	name       string
	r          Range
	nameRange  Range
	groupNames int
}

func (c editCompiler) resolveFunctionDecl(ctx context.Context, sel SymbolSelector) (*index, Symbol, parsedFile, *ast.FuncDecl, error) {
	idx, err := buildIndex(ctx, c.snapshot, core.SelectorScope(sel))
	if err != nil {
		return nil, Symbol{}, parsedFile{}, nil, err
	}
	sym, err := exactSymbol(idx, sel, "function")
	if err != nil {
		return nil, Symbol{}, parsedFile{}, nil, err
	}
	if sym.Kind != SymbolFunction && sym.Kind != SymbolMethod {
		return nil, Symbol{}, parsedFile{}, nil, errors.New("codegate: target must be a function or method")
	}
	src, err := c.snapshot.ReadFile(ctx, sym.Location.URI)
	if err != nil {
		return nil, Symbol{}, parsedFile{}, nil, err
	}
	pf, err := parseOne(sym.Location.URI, src)
	if err != nil {
		return nil, Symbol{}, parsedFile{}, nil, err
	}
	for _, decl := range pf.file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		qname := fn.Name.Name
		if fn.Recv != nil && len(fn.Recv.List) > 0 {
			qname = receiverName(fn.Recv.List[0].Type) + "." + fn.Name.Name
		}
		if qname == sym.QualifiedName {
			return idx, sym, pf, fn, nil
		}
	}
	return nil, Symbol{}, parsedFile{}, nil, errors.New("codegate: function declaration not found")
}

func (c editCompiler) resolveStructType(ctx context.Context, sel SymbolSelector) (Symbol, parsedFile, *ast.StructType, error) {
	idx, err := buildIndex(ctx, c.snapshot, core.SelectorScope(sel))
	if err != nil {
		return Symbol{}, parsedFile{}, nil, err
	}
	return c.resolveStructTypeFromIndex(ctx, idx, sel)
}

func (c editCompiler) resolveStructTypeFromIndex(ctx context.Context, idx *index, sel SymbolSelector) (Symbol, parsedFile, *ast.StructType, error) {
	if sel.Kind == "" {
		sel.Kind = SymbolStruct
	}
	sym, err := exactSymbol(idx, sel, "struct")
	if err != nil {
		return Symbol{}, parsedFile{}, nil, err
	}
	if sym.Kind != SymbolStruct {
		return Symbol{}, parsedFile{}, nil, errors.New("codegate: target must be a struct")
	}
	src, err := c.snapshot.ReadFile(ctx, sym.Location.URI)
	if err != nil {
		return Symbol{}, parsedFile{}, nil, err
	}
	pf, err := parseOne(sym.Location.URI, src)
	if err != nil {
		return Symbol{}, parsedFile{}, nil, err
	}
	for _, decl := range pf.file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || ts.Name.Name != sym.Name {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				return Symbol{}, parsedFile{}, nil, errors.New("codegate: target is not a struct")
			}
			return sym, pf, st, nil
		}
	}
	return Symbol{}, parsedFile{}, nil, errors.New("codegate: struct declaration not found")
}

func (c editCompiler) resolveInterfaceType(ctx context.Context, sel SymbolSelector) (Symbol, parsedFile, *ast.InterfaceType, error) {
	idx, err := buildIndex(ctx, c.snapshot, core.SelectorScope(sel))
	if err != nil {
		return Symbol{}, parsedFile{}, nil, err
	}
	return c.resolveInterfaceTypeFromIndex(ctx, idx, sel)
}

func (c editCompiler) resolveInterfaceTypeFromIndex(ctx context.Context, idx *index, sel SymbolSelector) (Symbol, parsedFile, *ast.InterfaceType, error) {
	if sel.Kind == "" {
		sel.Kind = SymbolInterface
	}
	sym, err := exactSymbol(idx, sel, "interface")
	if err != nil {
		return Symbol{}, parsedFile{}, nil, err
	}
	if sym.Kind != SymbolInterface {
		return Symbol{}, parsedFile{}, nil, errors.New("codegate: target must be an interface")
	}
	src, err := c.snapshot.ReadFile(ctx, sym.Location.URI)
	if err != nil {
		return Symbol{}, parsedFile{}, nil, err
	}
	pf, err := parseOne(sym.Location.URI, src)
	if err != nil {
		return Symbol{}, parsedFile{}, nil, err
	}
	for _, decl := range pf.file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || ts.Name.Name != sym.Name {
				continue
			}
			iface, ok := ts.Type.(*ast.InterfaceType)
			if !ok {
				return Symbol{}, parsedFile{}, nil, errors.New("codegate: target is not an interface")
			}
			return sym, pf, iface, nil
		}
	}
	return Symbol{}, parsedFile{}, nil, errors.New("codegate: interface declaration not found")
}

func structHasField(st *ast.StructType, fieldName string) bool {
	for _, field := range st.Fields.List {
		for _, name := range field.Names {
			if name.Name == fieldName {
				return true
			}
		}
	}
	return false
}

func findStructFieldName(st *ast.StructType, fieldName string) (*ast.Ident, error) {
	for _, field := range st.Fields.List {
		for _, name := range field.Names {
			if name.Name == fieldName {
				return name, nil
			}
		}
	}
	return nil, fmt.Errorf("codegate: field %q not found", fieldName)
}

func interfaceHasMethod(iface *ast.InterfaceType, methodName string) bool {
	for _, method := range iface.Methods.List {
		for _, name := range method.Names {
			if name.Name == methodName {
				return true
			}
		}
	}
	return false
}

func interfaceMethodName(method string) (string, error) {
	src := "package p\ntype T interface {\n" + method + "\n}\n"
	pf, err := parseOne("method.go", []byte(src))
	if err != nil {
		return "", err
	}
	for _, decl := range pf.file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			iface, ok := ts.Type.(*ast.InterfaceType)
			if !ok || len(iface.Methods.List) != 1 {
				continue
			}
			if len(iface.Methods.List[0].Names) != 1 {
				return "", errors.New("codegate: interface Method must declare exactly one named method")
			}
			return iface.Methods.List[0].Names[0].Name, nil
		}
	}
	return "", errors.New("codegate: invalid interface method")
}

func paramRanges(fset *token.FileSet, fields *ast.FieldList) []Range {
	if fields == nil {
		return nil
	}
	var out []Range
	for _, field := range fields.List {
		out = append(out, rangeOf(fset, field.Pos(), field.End()))
	}
	return out
}

func namedParamRanges(fset *token.FileSet, fields *ast.FieldList) []rangeItem {
	if fields == nil {
		return nil
	}
	var out []rangeItem
	for _, field := range fields.List {
		if len(field.Names) == 0 {
			continue
		}
		for _, name := range field.Names {
			out = append(out, rangeItem{name: name.Name, r: rangeOf(fset, field.Pos(), field.End()), nameRange: rangeOf(fset, name.Pos(), name.End()), groupNames: len(field.Names)})
		}
	}
	return out
}

func fieldRanges(fset *token.FileSet, fields *ast.FieldList) []Range {
	if fields == nil {
		return nil
	}
	var out []Range
	for _, field := range fields.List {
		out = append(out, rangeOf(fset, field.Pos(), field.End()))
	}
	return out
}

func insertListItemEdit(fset *token.FileSet, opening, closing token.Pos, items []Range, pos int, text string) TextEdit {
	if pos < 0 || pos > len(items) {
		pos = len(items)
	}
	if len(items) == 0 {
		offset := fset.Position(opening).Offset + 1
		return TextEdit{Range: Range{Start: Position{Offset: offset}, End: Position{Offset: offset}}, Replacement: text}
	}
	if pos == len(items) {
		offset := fset.Position(closing).Offset
		return TextEdit{Range: Range{Start: Position{Offset: offset}, End: Position{Offset: offset}}, Replacement: ", " + text}
	}
	offset := items[pos].Start.Offset
	return TextEdit{Range: Range{Start: Position{Offset: offset}, End: Position{Offset: offset}}, Replacement: text + ", "}
}

func insertStructFieldEdit(fset *token.FileSet, opening, closing token.Pos, items []Range, pos int, text string) TextEdit {
	if pos < 0 || pos > len(items) {
		pos = len(items)
	}
	if len(items) == 0 {
		offset := fset.Position(opening).Offset + 1
		return TextEdit{Range: Range{Start: Position{Offset: offset}, End: Position{Offset: offset}}, Replacement: "\n\t" + text + "\n"}
	}
	if pos == len(items) {
		offset := fset.Position(closing).Offset
		return TextEdit{Range: Range{Start: Position{Offset: offset}, End: Position{Offset: offset}}, Replacement: "\t" + text + "\n"}
	}
	offset := items[pos].Start.Offset
	return TextEdit{Range: Range{Start: Position{Offset: offset}, End: Position{Offset: offset}}, Replacement: text + "\n\t"}
}

func removeListItemRange(src []byte, r Range) Range {
	start, end := r.Start.Offset, r.End.Offset
	for end < len(src) && (src[end] == ' ' || src[end] == '\t') {
		end++
	}
	if end < len(src) && src[end] == ',' {
		end++
		for end < len(src) && src[end] == ' ' {
			end++
		}
		return Range{Start: Position{Offset: start}, End: Position{Offset: end}}
	}
	for start > 0 && src[start-1] == ' ' {
		start--
	}
	if start > 0 && src[start-1] == ',' {
		start--
	}
	return Range{Start: Position{Offset: start}, End: Position{Offset: end}}
}

func removeGroupedNameReplacement(src []byte, r Range) string {
	start := r.Start.Offset
	for start > 0 && src[start-1] == ' ' {
		start--
	}
	if start > 0 && src[start-1] == ',' {
		return " "
	}
	return ""
}

func namedFieldTypeRange(fset *token.FileSet, fields *ast.FieldList, name, label string) (Range, error) {
	if fields == nil {
		return Range{}, fmt.Errorf("codegate: %s %q not found", label, name)
	}
	for _, field := range fields.List {
		for _, candidate := range field.Names {
			if candidate.Name != name {
				continue
			}
			if len(field.Names) > 1 {
				return Range{}, fmt.Errorf("codegate: cannot change type for grouped %s %q", label, name)
			}
			if field.Type == nil {
				return Range{}, fmt.Errorf("codegate: %s %q has no type", label, name)
			}
			return rangeOf(fset, field.Type.Pos(), field.Type.End()), nil
		}
	}
	return Range{}, fmt.Errorf("codegate: %s %q not found", label, name)
}

func resultTypeRange(fset *token.FileSet, fields *ast.FieldList, pos int) (Range, error) {
	if fields == nil || pos < 0 {
		return Range{}, fmt.Errorf("codegate: result position %d not found", pos)
	}
	i := 0
	for _, field := range fields.List {
		count := 1
		if len(field.Names) > 0 {
			count = len(field.Names)
		}
		for n := 0; n < count; n++ {
			if i == pos {
				if count > 1 {
					return Range{}, fmt.Errorf("codegate: cannot change type for grouped result at position %d", pos)
				}
				if field.Type == nil {
					return Range{}, fmt.Errorf("codegate: result at position %d has no type", pos)
				}
				return rangeOf(fset, field.Type.Pos(), field.Type.End()), nil
			}
			i++
		}
	}
	return Range{}, fmt.Errorf("codegate: result position %d not found", pos)
}

func (c editCompiler) structFieldUsageEdits(ctx context.Context, idx *index, sym Symbol, oldName, newName string, updateSelectors bool) ([]FileEdit, error) {
	byPath := map[string][]TextEdit{}
	for _, file := range idx.unitFiles[sym.UnitID] {
		src, err := c.snapshot.ReadFile(ctx, file)
		if err != nil {
			return nil, err
		}
		pf, err := parseOne(file, src)
		if err != nil {
			return nil, err
		}
		ast.Inspect(pf.file, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.CompositeLit:
				if !isCompositeTypeName(x.Type, sym.Name) {
					return true
				}
				for _, elt := range x.Elts {
					kv, ok := elt.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					key, ok := kv.Key.(*ast.Ident)
					if ok && key.Name == oldName {
						byPath[file] = append(byPath[file], TextEdit{Path: file, Range: rangeOf(pf.fset, key.Pos(), key.End()), Replacement: newName})
					}
				}
			case *ast.SelectorExpr:
				if updateSelectors && x.Sel.Name == oldName {
					byPath[file] = append(byPath[file], TextEdit{Path: file, Range: rangeOf(pf.fset, x.Sel.Pos(), x.Sel.End()), Replacement: newName})
				}
			}
			return true
		})
	}
	var out []FileEdit
	for p, edits := range byPath {
		out = append(out, FileEdit{Path: p, Edits: edits})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func isCompositeTypeName(expr ast.Expr, name string) bool {
	switch x := expr.(type) {
	case *ast.Ident:
		return x.Name == name
	case *ast.SelectorExpr:
		return x.Sel.Name == name
	case *ast.StarExpr:
		return isCompositeTypeName(x.X, name)
	default:
		return false
	}
}

func fieldNameAmbiguous(idx *index, owner Symbol, fieldName string) bool {
	count := 0
	for _, sym := range idx.symbols {
		if sym.UnitID == owner.UnitID && sym.Kind == SymbolField && sym.Name == fieldName {
			count++
			if count > 1 {
				return true
			}
		}
	}
	return false
}

func hasSelectorUse(ctx context.Context, snapshot Snapshot, idx *index, unitID, name string) bool {
	for _, file := range idx.unitFiles[unitID] {
		src, err := snapshot.ReadFile(ctx, file)
		if err != nil {
			continue
		}
		pf, err := parseOne(file, src)
		if err != nil {
			continue
		}
		found := false
		ast.Inspect(pf.file, func(n ast.Node) bool {
			if found {
				return false
			}
			x, ok := n.(*ast.SelectorExpr)
			if ok && x.Sel.Name == name {
				found = true
				return false
			}
			return true
		})
		if found {
			return true
		}
	}
	return false
}

func validateAddParameterName(fn *ast.FuncDecl, name string) error {
	for _, existing := range functionParameterNames(fn) {
		if existing == name {
			return fmt.Errorf("codegate: parameter %q already exists", name)
		}
	}
	if functionBodyDeclares(fn, name) {
		return fmt.Errorf("codegate: parameter %q would shadow a local declaration", name)
	}
	return nil
}

func validateRenameParameterName(fn *ast.FuncDecl, oldName, newName string) error {
	for _, existing := range functionParameterNames(fn) {
		if existing == newName && existing != oldName {
			return fmt.Errorf("codegate: parameter %q already exists", newName)
		}
	}
	if functionBodyDeclares(fn, oldName) {
		return fmt.Errorf("codegate: parameter %q is shadowed in the function body", oldName)
	}
	if functionBodyDeclares(fn, newName) {
		return fmt.Errorf("codegate: parameter %q would shadow a local declaration", newName)
	}
	return nil
}

func functionParameterNames(fn *ast.FuncDecl) []string {
	if fn.Type == nil || fn.Type.Params == nil {
		return nil
	}
	var out []string
	for _, field := range fn.Type.Params.List {
		for _, name := range field.Names {
			out = append(out, name.Name)
		}
	}
	return out
}

func functionBodyDeclares(fn *ast.FuncDecl, name string) bool {
	if fn.Body == nil {
		return false
	}
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if found {
			return false
		}
		switch x := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.AssignStmt:
			if x.Tok == token.DEFINE {
				for _, expr := range x.Lhs {
					if id, ok := expr.(*ast.Ident); ok && id.Name == name {
						found = true
						return false
					}
				}
			}
		case *ast.RangeStmt:
			if x.Tok == token.DEFINE {
				if id, ok := x.Key.(*ast.Ident); ok && id.Name == name {
					found = true
					return false
				}
				if id, ok := x.Value.(*ast.Ident); ok && id.Name == name {
					found = true
					return false
				}
			}
		case *ast.ValueSpec:
			for _, id := range x.Names {
				if id.Name == name {
					found = true
					return false
				}
			}
		}
		return true
	})
	return found
}

func (c editCompiler) ensureFunctionSignatureSafe(ctx context.Context, idx *index, target Symbol, fn *ast.FuncDecl, allowVariadic bool) error {
	if !allowVariadic && functionIsVariadic(fn) {
		return fmt.Errorf("codegate: cannot update call sites for variadic function %q", target.Name)
	}
	for _, occ := range idx.occurrences {
		if occ.SymbolID != target.ID {
			continue
		}
		switch occ.Kind {
		case OccurrenceRead, OccurrenceWrite, OccurrenceReference:
			return fmt.Errorf("codegate: cannot change signature for %q because it is used as a function value", target.Name)
		}
	}
	if target.Kind == SymbolMethod && methodNameAmbiguous(idx, target) {
		return fmt.Errorf("codegate: method %q selector resolution is ambiguous", target.Name)
	}
	for _, file := range idx.unitFiles[target.UnitID] {
		src, err := c.snapshot.ReadFile(ctx, file)
		if err != nil {
			return err
		}
		pf, err := parseOne(file, src)
		if err != nil {
			return err
		}
		var guardErr error
		ast.Inspect(pf.file, func(n ast.Node) bool {
			if guardErr != nil {
				return false
			}
			call, ok := n.(*ast.CallExpr)
			if !ok || callExprName(call.Fun) != target.Name {
				return true
			}
			callee := callTarget(idx, target.UnitID, call.Fun)
			if callee.ID == "" {
				guardErr = fmt.Errorf("codegate: cannot change signature for %q because a call site is unresolved", target.Name)
				return false
			}
			if target.Kind == SymbolMethod && callee.ID != target.ID {
				guardErr = fmt.Errorf("codegate: cannot change signature for %q because selector call sites are ambiguous", target.Name)
				return false
			}
			if callee.ID == target.ID && call.Ellipsis.IsValid() {
				guardErr = fmt.Errorf("codegate: cannot change signature for %q because a call site uses variadic expansion", target.Name)
				return false
			}
			return true
		})
		if guardErr != nil {
			return guardErr
		}
	}
	return nil
}

func functionIsVariadic(fn *ast.FuncDecl) bool {
	if fn.Type == nil || fn.Type.Params == nil || len(fn.Type.Params.List) == 0 {
		return false
	}
	_, ok := fn.Type.Params.List[len(fn.Type.Params.List)-1].Type.(*ast.Ellipsis)
	return ok
}

func methodNameAmbiguous(idx *index, target Symbol) bool {
	count := 0
	for _, sym := range idx.symbols {
		if sym.UnitID == target.UnitID && sym.Kind == SymbolMethod && sym.Name == target.Name {
			count++
			if count > 1 {
				return true
			}
		}
	}
	return false
}

func callExprName(expr ast.Expr) string {
	switch x := expr.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return x.Sel.Name
	default:
		return ""
	}
}

func (c editCompiler) callArgumentEdits(ctx context.Context, idx *index, target Symbol, pos int, value string, add bool) ([]FileEdit, error) {
	byPath := map[string][]TextEdit{}
	for _, file := range idx.unitFiles[target.UnitID] {
		src, err := c.snapshot.ReadFile(ctx, file)
		if err != nil {
			return nil, err
		}
		pf, err := parseOne(file, src)
		if err != nil {
			return nil, err
		}
		ast.Inspect(pf.file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			callee := callTarget(idx, target.UnitID, call.Fun)
			if callee.ID != target.ID {
				return true
			}
			if add {
				edit := insertListItemEdit(pf.fset, call.Lparen, call.Rparen, exprRanges(pf.fset, call.Args), pos, value)
				edit.Path = file
				byPath[file] = append(byPath[file], edit)
				return true
			}
			if pos < 0 || pos >= len(call.Args) {
				byPath[file] = append(byPath[file], TextEdit{Path: file, Range: Range{Start: Position{Offset: -1}}, Replacement: ""})
				return true
			}
			r := rangeOf(pf.fset, call.Args[pos].Pos(), call.Args[pos].End())
			byPath[file] = append(byPath[file], TextEdit{Path: file, Range: removeListItemRange(src, r), Replacement: ""})
			return true
		})
	}
	var out []FileEdit
	for p, edits := range byPath {
		for _, edit := range edits {
			if edit.Range.Start.Offset < 0 {
				return nil, errors.New("codegate: call site has too few arguments")
			}
		}
		out = append(out, FileEdit{Path: p, Edits: edits})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func exprRanges(fset *token.FileSet, exprs []ast.Expr) []Range {
	out := make([]Range, 0, len(exprs))
	for _, expr := range exprs {
		out = append(out, rangeOf(fset, expr.Pos(), expr.End()))
	}
	return out
}

func enclosingFunction(idx *index, p string, start, end int) Symbol {
	var best Symbol
	for _, sym := range idx.symbols {
		if sym.Location.URI != p || (sym.Kind != SymbolFunction && sym.Kind != SymbolMethod) {
			continue
		}
		if sym.BodyRange.Start.Offset <= start && end <= sym.BodyRange.End.Offset {
			if best.ID == "" || sym.BodyRange.Start.Offset > best.BodyRange.Start.Offset {
				best = sym
			}
		}
	}
	return best
}

func formatExtractedDecl(receiver, name, params, results, body string) string {
	head := "func "
	if strings.TrimSpace(receiver) != "" {
		head += "(" + strings.TrimSpace(receiver) + ") "
	}
	head += name + "(" + strings.TrimSpace(params) + ")"
	if strings.TrimSpace(results) != "" {
		head += " " + strings.TrimSpace(results)
	}
	return head + " {\n" + body + "\n}"
}

func (c editCompiler) moveImportEdits(ctx context.Context, sym Symbol, sourceText, toPath string) ([]FileEdit, error) {
	src, err := c.snapshot.ReadFile(ctx, sym.Location.URI)
	if err != nil {
		return nil, err
	}
	target, err := c.snapshot.ReadFile(ctx, toPath)
	if err != nil {
		return nil, err
	}
	sourcePF, err := parseOne(sym.Location.URI, src)
	if err != nil {
		return nil, err
	}
	targetPF, err := parseOne(toPath, target)
	if err != nil {
		return nil, err
	}
	movedQualifiers := selectorQualifierNames(sourceText)
	if len(movedQualifiers) == 0 {
		return nil, nil
	}
	deleteRange := deleteRangeForSymbol(src, sym)
	remaining := append([]byte(nil), src[:deleteRange.Start.Offset]...)
	remaining = append(remaining, src[deleteRange.End.Offset:]...)
	remainingQualifiers := selectorQualifierNames(string(remaining))

	var targetImports []importNeed
	var sourceRemovals []TextEdit
	for _, imp := range sourcePF.file.Imports {
		importPath, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return nil, err
		}
		alias := importAlias(imp)
		local := importLocalName(importPath, alias)
		if local == "" || !movedQualifiers[local] {
			continue
		}
		if findImportSpec(targetPF, importPath, alias) == nil {
			targetImports = append(targetImports, importNeed{path: importPath, alias: alias})
		}
		if !remainingQualifiers[local] {
			sourceRemovals = append(sourceRemovals, TextEdit{
				Path:        sym.Location.URI,
				Range:       removeImportRange(sourcePF, imp),
				Replacement: "",
			})
		}
	}
	var out []FileEdit
	if len(targetImports) > 0 {
		edit, err := ensureImportsEdit(targetPF, targetImports)
		if err != nil {
			return nil, err
		}
		out = append(out, FileEdit{Path: toPath, Edits: []TextEdit{edit}})
	}
	if len(sourceRemovals) > 0 {
		out = append(out, FileEdit{Path: sym.Location.URI, Edits: sourceRemovals})
	}
	return out, nil
}

type importNeed struct {
	path  string
	alias string
}

func importLocalName(importPath, alias string) string {
	if alias == "." || alias == "_" {
		return ""
	}
	if alias != "" {
		return alias
	}
	importPath = strings.TrimRight(importPath, "/")
	parts := strings.Split(importPath, "/")
	name := parts[len(parts)-1]
	if semanticImportVersion(name) && len(parts) >= 2 {
		name = parts[len(parts)-2]
	}
	if dot := strings.LastIndex(name, ".v"); dot > 0 && semanticImportVersion(name[dot+1:]) {
		name = name[:dot]
	}
	name = goModulePackageName(name)
	if !isValidIdentifier(name) {
		return ""
	}
	return name
}

func goModulePackageName(name string) string {
	name = strings.TrimPrefix(name, "go-")
	name = strings.TrimSuffix(name, "-go")
	return name
}

func semanticImportVersion(segment string) bool {
	if len(segment) < 2 || segment[0] != 'v' {
		return false
	}
	if segment[1] < '2' || segment[1] > '9' {
		return false
	}
	for i := 2; i < len(segment); i++ {
		if segment[i] < '0' || segment[i] > '9' {
			return false
		}
	}
	return true
}

func selectorQualifierNames(src string) map[string]bool {
	fset := token.NewFileSet()
	parseSrc := src
	if !strings.HasPrefix(strings.TrimSpace(src), "package ") {
		parseSrc = "package snippet\n\n" + src
	}
	file, err := parser.ParseFile(fset, "snippet.go", parseSrc, 0)
	if err != nil {
		return identifierSet(src)
	}
	out := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if ok {
			out[id.Name] = true
		}
		return true
	})
	return out
}

func identifierSet(src string) map[string]bool {
	out := map[string]bool{}
	for i := 0; i < len(src); {
		r, size := rune(src[i]), 1
		if r >= utf8.RuneSelf {
			r, size = utf8.DecodeRuneInString(src[i:])
		}
		if r != '_' && !unicode.IsLetter(r) {
			i += size
			continue
		}
		start := i
		for i < len(src) {
			r = rune(src[i])
			size = 1
			if r >= utf8.RuneSelf {
				r, size = utf8.DecodeRuneInString(src[i:])
			}
			if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
				break
			}
			i += size
		}
		out[src[start:i]] = true
	}
	return out
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
	return ensureImportsEdit(pf, []importNeed{{path: importPath, alias: alias}})
}

func ensureImportsEdit(pf parsedFile, imports []importNeed) (TextEdit, error) {
	specs := make([]string, 0, len(imports))
	seen := map[string]bool{}
	for _, imp := range imports {
		if err := validateImportAlias(imp.alias); err != nil {
			return TextEdit{}, err
		}
		key := imp.alias + "\x00" + imp.path
		if seen[key] {
			continue
		}
		seen[key] = true
		specs = append(specs, formatImportSpec(imp.path, imp.alias))
	}
	if len(specs) == 0 {
		return TextEdit{}, errors.New("codegate: no imports to ensure")
	}
	decls := importDecls(pf)
	if len(decls) == 0 {
		offset := pf.fset.Position(pf.file.Name.End()).Offset
		replacement := "\n\nimport " + specs[0]
		if len(specs) > 1 {
			replacement = "\n\nimport (\n\t" + strings.Join(specs, "\n\t") + "\n)"
		}
		return TextEdit{
			Path:        pf.path,
			Range:       Range{Start: Position{Offset: offset}, End: Position{Offset: offset}},
			Replacement: replacement,
		}, nil
	}
	gen := decls[len(decls)-1]
	if gen.Lparen.IsValid() {
		offset := pf.fset.Position(gen.Rparen).Offset
		return TextEdit{
			Path:        pf.path,
			Range:       Range{Start: Position{Offset: offset}, End: Position{Offset: offset}},
			Replacement: "\n\t" + strings.Join(specs, "\n\t"),
		}, nil
	}
	if len(gen.Specs) != 1 {
		return TextEdit{}, errors.New("codegate: malformed import declaration")
	}
	existing := strings.TrimSpace(sourceSlice(pf.src, pf.fset, gen.Specs[0].Pos(), gen.Specs[0].End()))
	replacement := "import (\n\t" + existing + "\n\t" + strings.Join(specs, "\n\t") + "\n)"
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
		return fmt.Errorf("codegate: invalid Go import alias %q", alias)
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
	return 0, 0, errors.New("codegate: declaration not found for comment replacement")
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
				return nil, errors.New("codegate: target is not a struct")
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
	return nil, errors.New("codegate: struct field not found")
}

func tagEditOffsets(pf parsedFile, field *ast.Field) (int, int) {
	if field.Tag != nil {
		return pf.fset.Position(field.Tag.Pos()).Offset, pf.fset.Position(field.Tag.End()).Offset
	}
	return pf.fset.Position(field.End()).Offset, pf.fset.Position(field.End()).Offset
}
