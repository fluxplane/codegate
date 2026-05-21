package goast

import (
	"context"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/scanner"
	"go/token"
	"go/types"
	"sort"
	"strings"

	"github.com/codewandler/editor/internal/core"
)

func (b GoBackend) Validate(ctx context.Context, snapshot Snapshot, opts ValidationOptions) (ValidationResult, error) {
	kinds := validationKinds(opts.Kinds)
	result := ValidationResult{
		Passed:         true,
		Kinds:          append([]ValidationKind(nil), kinds...),
		ResolutionMode: "ast",
		Complete:       true,
	}
	files, err := goFiles(ctx, snapshot, opts.Scope)
	if err != nil {
		return ValidationResult{}, err
	}
	result.AffectedPaths = append(result.AffectedPaths, files...)

	parsed, parseDiagnostics, err := parseValidationFiles(ctx, snapshot, files)
	if err != nil {
		return ValidationResult{}, err
	}
	for _, kind := range kinds {
		switch kind {
		case ValidationParse:
			result.Diagnostics = append(result.Diagnostics, parseDiagnostics...)
		case ValidationTypecheck:
			result.ResolutionMode = "typecheck"
			if len(parseDiagnostics) > 0 {
				result.Diagnostics = append(result.Diagnostics, parseDiagnostics...)
				continue
			}
			modulePath, err := readModulePath(ctx, snapshot)
			if err != nil {
				return ValidationResult{}, err
			}
			result.Diagnostics = append(result.Diagnostics, typecheckDiagnostics(parsed, modulePath)...)
		default:
			result.Complete = false
			result.Diagnostics = append(result.Diagnostics, Diagnostic{Severity: "warning", Message: fmt.Sprintf("unsupported Go validation kind %q", kind)})
		}
	}
	result.Diagnostics = dedupeDiagnostics(result.Diagnostics)
	if len(result.Diagnostics) > 0 {
		result.Passed = false
	}
	sort.Strings(result.AffectedPaths)
	return result, nil
}

func validationKinds(kinds []ValidationKind) []ValidationKind {
	if len(kinds) == 0 {
		return []ValidationKind{ValidationParse}
	}
	seen := map[ValidationKind]bool{}
	out := make([]ValidationKind, 0, len(kinds))
	for _, kind := range kinds {
		if kind == "" || seen[kind] {
			continue
		}
		seen[kind] = true
		out = append(out, kind)
	}
	if len(out) == 0 {
		return []ValidationKind{ValidationParse}
	}
	return out
}

func goFiles(ctx context.Context, snapshot Snapshot, scope Scope) ([]string, error) {
	if scope.Language != "" && scope.Language != Go {
		return nil, nil
	}
	files, err := snapshot.ListFiles(ctx, scope)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(files))
	for _, p := range files {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !strings.HasSuffix(p, ".go") || (!scope.IncludeTests && core.HasTestPath(p)) {
			continue
		}
		out = append(out, core.CleanPath(p))
	}
	sort.Strings(out)
	return out, nil
}

type validationFile struct {
	path string
	src  []byte
	fset *token.FileSet
	file *ast.File
}

type LimitationCounts struct {
	SelectorCalls  int
	ComplexCallees int
	VariadicCalls  int
}

func CallSiteLimitations(ctx context.Context, snapshot Snapshot, scope Scope) (LimitationCounts, error) {
	files, err := goFiles(ctx, snapshot, scope)
	if err != nil {
		return LimitationCounts{}, err
	}
	var counts LimitationCounts
	for _, p := range files {
		src, err := snapshot.ReadFile(ctx, p)
		if err != nil {
			return LimitationCounts{}, err
		}
		file, err := parser.ParseFile(token.NewFileSet(), p, src, 0)
		if err != nil {
			continue
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if call.Ellipsis.IsValid() {
				counts.VariadicCalls++
			}
			switch call.Fun.(type) {
			case *ast.Ident:
			case *ast.SelectorExpr:
				counts.SelectorCalls++
			default:
				counts.ComplexCallees++
			}
			return true
		})
	}
	return counts, nil
}

func parseValidationFiles(ctx context.Context, snapshot Snapshot, paths []string) ([]validationFile, []Diagnostic, error) {
	var parsed []validationFile
	var diagnostics []Diagnostic
	for _, p := range paths {
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		src, err := snapshot.ReadFile(ctx, p)
		if err != nil {
			return nil, nil, err
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, p, src, parser.ParseComments|parser.AllErrors)
		if err != nil {
			diagnostics = append(diagnostics, parseErrorDiagnostics(fset, p, err)...)
			continue
		}
		parsed = append(parsed, validationFile{path: p, src: src, fset: fset, file: file})
	}
	return parsed, diagnostics, nil
}

func parseErrorDiagnostics(fset *token.FileSet, fallbackPath string, err error) []Diagnostic {
	switch x := err.(type) {
	case scanner.ErrorList:
		out := make([]Diagnostic, 0, len(x))
		for _, e := range x {
			out = append(out, diagnosticFromPosition(e.Pos, "error", e.Msg))
		}
		return out
	case *scanner.ErrorList:
		out := make([]Diagnostic, 0, len(*x))
		for _, e := range *x {
			out = append(out, diagnosticFromPosition(e.Pos, "error", e.Msg))
		}
		return out
	default:
		pos := token.Position{Filename: fallbackPath}
		if f := fset.File(token.Pos(1)); f != nil {
			pos = f.Position(token.Pos(1))
		}
		return []Diagnostic{diagnosticFromPosition(pos, "error", err.Error())}
	}
}

type validationPackage struct {
	unit       string
	name       string
	importPath string
	files      []validationFile
}

func typecheckDiagnostics(files []validationFile, modulePath string) []Diagnostic {
	packages := validationPackages(files, modulePath)
	externalImports := externalImportPaths(files, modulePath)
	externalAliases := externalImportAliases(files, modulePath)
	importer := &validationImporter{
		packages:        packages,
		externalImports: externalImports,
		externalAliases: externalAliases,
		std:             importer.Default(),
		checked:         map[string]*types.Package{},
		checking:        map[string]bool{},
	}
	units := make([]string, 0, len(packages))
	for unit := range packages {
		units = append(units, unit)
	}
	sort.Strings(units)
	for _, unit := range units {
		importer.check(unit)
	}
	return importer.diagnostics
}

func validationPackages(files []validationFile, modulePath string) map[string]validationPackage {
	byUnit := map[string]validationPackage{}
	for _, vf := range files {
		unit := unitID(vf.path, vf.file.Name.Name)
		pkg := byUnit[unit]
		if pkg.unit == "" {
			pkg.unit = unit
			pkg.name = vf.file.Name.Name
			pkg.importPath = packageImportPath(dirFromFilePath(vf.path), modulePath)
		}
		pkg.files = append(pkg.files, vf)
		byUnit[unit] = pkg
	}
	return byUnit
}

func packageImportPath(dir, modulePath string) string {
	if modulePath == "" {
		return dir
	}
	if dir == "." {
		return modulePath
	}
	return modulePath + "/" + dir
}

func dirFromFilePath(p string) string {
	p = core.CleanPath(p)
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[:i]
	}
	return "."
}

func readModulePath(ctx context.Context, snapshot Snapshot) (string, error) {
	src, err := snapshot.ReadFile(ctx, "go.mod")
	if err != nil {
		return "", nil
	}
	for _, line := range strings.Split(string(src), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")), nil
		}
	}
	return "", nil
}

type validationImporter struct {
	packages        map[string]validationPackage
	externalImports map[string]bool
	externalAliases map[string]bool
	std             types.Importer
	checked         map[string]*types.Package
	checking        map[string]bool
	diagnostics     []Diagnostic
}

func (i *validationImporter) Import(path string) (*types.Package, error) {
	for unit, pkg := range i.packages {
		if pkg.importPath == path {
			return i.check(unit), nil
		}
	}
	pkg, err := i.std.Import(path)
	if err != nil && i.externalImports[path] {
		return types.NewPackage(path, pathBase(path)), nil
	}
	return pkg, err
}

func (i *validationImporter) check(unit string) *types.Package {
	pkgInfo, ok := i.packages[unit]
	if !ok {
		return types.NewPackage(unit, pathBase(unit))
	}
	if pkg := i.checked[unit]; pkg != nil {
		return pkg
	}
	if i.checking[unit] {
		return types.NewPackage(pkgInfo.importPath, pkgInfo.name)
	}
	i.checking[unit] = true
	defer delete(i.checking, unit)

	fset := token.NewFileSet()
	asts := make([]*ast.File, 0, len(pkgInfo.files))
	for _, vf := range pkgInfo.files {
		af, err := parser.ParseFile(fset, vf.path, vf.src, parser.ParseComments|parser.AllErrors)
		if err != nil {
			i.diagnostics = append(i.diagnostics, parseErrorDiagnostics(fset, vf.path, err)...)
			continue
		}
		asts = append(asts, af)
	}
	if len(asts) == 0 {
		pkg := types.NewPackage(pkgInfo.importPath, pkgInfo.name)
		i.checked[unit] = pkg
		return pkg
	}
	collected := 0
	cfg := types.Config{
		Importer: i,
		Error: func(err error) {
			if ignoredExternalTypeError(err, i.externalAliases) {
				return
			}
			collected++
			i.diagnostics = append(i.diagnostics, typeErrorDiagnostic(err))
		},
	}
	pkg, err := cfg.Check(pkgInfo.importPath, fset, asts, nil)
	if err != nil && collected == 0 && !ignoredExternalTypeError(err, i.externalAliases) {
		i.diagnostics = append(i.diagnostics, typeErrorDiagnostic(err))
	}
	if pkg == nil {
		pkg = types.NewPackage(pkgInfo.importPath, pkgInfo.name)
	}
	i.checked[unit] = pkg
	return pkg
}

func externalImportPaths(files []validationFile, modulePath string) map[string]bool {
	out := map[string]bool{}
	for _, vf := range files {
		for _, imp := range vf.file.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			if isExternalModuleImport(importPath, modulePath) {
				out[importPath] = true
			}
		}
	}
	return out
}

func externalImportAliases(files []validationFile, modulePath string) map[string]bool {
	out := map[string]bool{}
	for _, vf := range files {
		for _, imp := range vf.file.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			if !isExternalModuleImport(importPath, modulePath) {
				continue
			}
			alias := importAlias(imp)
			local := importLocalName(importPath, alias)
			if local != "" {
				out[local] = true
			}
		}
	}
	return out
}

func isExternalModuleImport(importPath, modulePath string) bool {
	if modulePath != "" && (importPath == modulePath || strings.HasPrefix(importPath, modulePath+"/")) {
		return false
	}
	first := importPath
	if i := strings.IndexByte(first, '/'); i >= 0 {
		first = first[:i]
	}
	return strings.Contains(first, ".")
}

func ignoredExternalTypeError(err error, aliases map[string]bool) bool {
	te, ok := err.(types.Error)
	if !ok {
		return false
	}
	const prefix = "undefined: "
	if !strings.HasPrefix(te.Msg, prefix) {
		return false
	}
	name := strings.TrimPrefix(te.Msg, prefix)
	for alias := range aliases {
		if name == alias || strings.HasPrefix(name, alias+".") {
			return true
		}
	}
	return false
}

func pathBase(p string) string {
	p = strings.TrimRight(p, "/")
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	if i := strings.LastIndexByte(p, '#'); i >= 0 {
		return p[i+1:]
	}
	return p
}

func typeErrorDiagnostic(err error) Diagnostic {
	if te, ok := err.(types.Error); ok {
		return diagnosticFromPosition(te.Fset.Position(te.Pos), "error", te.Msg)
	}
	return Diagnostic{Severity: "error", Message: err.Error()}
}

func diagnosticFromPosition(pos token.Position, severity, message string) Diagnostic {
	return Diagnostic{
		Severity: severity,
		Message:  message,
		Location: Location{
			URI: core.CleanPath(pos.Filename),
			Range: Range{
				Start: Position{Line: pos.Line, Column: pos.Column, Offset: pos.Offset},
				End:   Position{Line: pos.Line, Column: pos.Column, Offset: pos.Offset},
			},
		},
	}
}

func dedupeDiagnostics(in []Diagnostic) []Diagnostic {
	seen := map[string]bool{}
	out := make([]Diagnostic, 0, len(in))
	for _, d := range in {
		key := fmt.Sprintf("%s\x00%d\x00%d\x00%s", d.Location.URI, d.Location.Range.Start.Line, d.Location.Range.Start.Column, d.Message)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, d)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Location.URI != out[j].Location.URI {
			return out[i].Location.URI < out[j].Location.URI
		}
		if out[i].Location.Range.Start.Offset != out[j].Location.Range.Start.Offset {
			return out[i].Location.Range.Start.Offset < out[j].Location.Range.Start.Offset
		}
		return out[i].Message < out[j].Message
	})
	return out
}
