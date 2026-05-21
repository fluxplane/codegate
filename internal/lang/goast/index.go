package goast

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/codewandler/codegate/internal/core"
)

type GoBackend struct{}

func New() GoBackend {
	return GoBackend{}
}

func (b GoBackend) Spec() BackendSpec {
	assessment := goAssessmentSupport()
	return BackendSpec{
		Language:       Go,
		Name:           "goast",
		FileExtensions: []string{".go"},
		Capabilities: []CapabilitySupport{
			{Capability: CapabilityLookup, Level: CapabilityAdvanced, Notes: "AST-backed symbol, position, reference, and call lookup for Go source."},
			{Capability: CapabilityStaticAnalysis, Level: CapabilityAdvanced, Notes: "Indexes packages, symbols, imports, references, calls, and implementation edges."},
			{Capability: CapabilityQuality, Level: CapabilityBasic, Notes: "Computes pressure, debt, complexity, shape, safety, and performance smell metrics."},
			{Capability: CapabilityEditing, Level: CapabilityAdvanced, Notes: "Compiles structured Go edit operations into formatted source changes."},
			{Capability: CapabilityRefactoring, Level: CapabilityBasic, Notes: "Supports executable low-risk operations plus advisory higher-level proposals."},
			{Capability: CapabilityValidation, Level: CapabilityBasic, Notes: "Runs parse checks and best-effort type checking for local module code."},
			{Capability: CapabilityReporting, Level: CapabilityBasic, Notes: "Feeds package, metric, diagnostic, and proposal data into assessment reports."},
		},
		Operations: OperationSupport{
			Lookup:          []string{"symbol", "qualified_name", "position", "references", "callers", "callees"},
			AssessmentGates: assessment.Gates,
			Assessment:      assessment,
			ValidationKinds: []ValidationKind{ValidationParse, ValidationTypecheck},
			EditOperations: []OperationKind{
				OpReplaceFunction, OpAppendFunction, OpDeleteFunction, OpReplaceMethod, OpDeleteMethod,
				OpReplaceSymbol, OpAppendSymbol, OpDeleteSymbol, OpReplaceComment, OpRenameSymbol,
				OpEnsureStructTag, OpRemoveStructTag, OpEnsureGoImport, OpRemoveGoImport, OpRenameGoImport,
				OpRenameGoModulePath, OpMoveSymbol, OpAddGoParameter, OpRemoveGoParam, OpRenameGoParam, OpAddGoField,
				OpRemoveGoField, OpRenameGoField, OpChangeGoParam, OpChangeGoResult, OpRenameGoRecv,
				OpAddGoIfaceMeth, OpRemoveGoIface, OpExtractGoFunc, OpExtractGoMethod,
			},
			RefactorKinds: []RefactorKind{RefactorDeleteSymbol, RefactorExtractFunction, RefactorIntroduceConfig, RefactorSplitFunction, RefactorSplitPackage, RefactorReplaceFlagArgument, RefactorReviewDebtMarkers},
			Notes:         []string{"Go edits are AST-backed and formatted in memory; typecheck validation is best-effort."},
		},
		ResolutionMode: "ast",
	}
}

func (b GoBackend) Index(ctx context.Context, snapshot Snapshot, scope Scope) (*Index, error) {
	idx, err := buildIndex(ctx, snapshot, scope)
	if err != nil {
		return nil, err
	}
	return exportIndex(idx), nil
}

type index struct {
	documents   []Document
	packages    []PackageInfo
	symbols     []Symbol
	occurrences []Occurrence
	edges       []Edge
	imports     []ImportEdge
	diagnostics []Diagnostic
	debtMarkers []core.DebtMarker
	quality     goQualityReport
	byID        map[SymbolID]Symbol
	byName      map[string][]Symbol
	unitFiles   map[string][]string
	fileUnits   map[string]string
	fileLOC     map[string]int
}

type parsedFile struct {
	path string
	src  []byte
	fset *token.FileSet
	file *ast.File
	unit string
}

func buildIndex(ctx context.Context, snapshot Snapshot, scope Scope) (*index, error) {
	if scope.Language != "" && scope.Language != Go {
		return &index{
			byID:      map[SymbolID]Symbol{},
			byName:    map[string][]Symbol{},
			unitFiles: map[string][]string{},
			fileUnits: map[string]string{},
		}, nil
	}
	files, err := goFiles(ctx, snapshot, scope)
	if err != nil {
		return nil, err
	}
	idx := &index{
		byID:      map[SymbolID]Symbol{},
		byName:    map[string][]Symbol{},
		unitFiles: map[string][]string{},
		fileUnits: map[string]string{},
		fileLOC:   map[string]int{},
	}
	var parsed []parsedFile
	for _, p := range files {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		src, err := snapshot.ReadFile(ctx, p)
		if err != nil {
			idx.diagnostics = append(idx.diagnostics, Diagnostic{Severity: "error", Message: err.Error()})
			continue
		}
		if scope.MaxBytes > 0 && int64(len(src)) > scope.MaxBytes {
			continue
		}
		fset := token.NewFileSet()
		af, err := parser.ParseFile(fset, p, src, parser.ParseComments)
		if err != nil {
			idx.diagnostics = append(idx.diagnostics, Diagnostic{Severity: "error", Message: err.Error()})
			continue
		}
		unit := unitID(p, af.Name.Name)
		if scope.UnitID != "" && unit != scope.UnitID {
			continue
		}
		pf := parsedFile{path: p, src: src, fset: fset, file: af, unit: unit}
		parsed = append(parsed, pf)
	}
	if scope.MaxFiles > 0 && len(parsed) > scope.MaxFiles {
		parsed = parsed[:scope.MaxFiles]
	}
	for _, pf := range parsed {
		idx.documents = append(idx.documents, Document{URI: pf.path, Language: Go, UnitID: pf.unit})
		idx.unitFiles[pf.unit] = append(idx.unitFiles[pf.unit], pf.path)
		idx.fileUnits[pf.path] = pf.unit
		idx.fileLOC[pf.path] = countLines(pf.src)
		idx.debtMarkers = append(idx.debtMarkers, goDebtMarkers(pf.path, pf.src, pf.fset, pf.file)...)
		idx.quality.merge(collectGoQuality(pf))
	}
	idx.quality.finalize(scope.IncludeTests)
	for _, pf := range parsed {
		indexDecls(idx, pf)
		indexImports(idx, pf)
	}
	for _, pf := range parsed {
		indexUses(idx, pf)
	}
	indexImplementations(idx)
	indexPackages(idx)
	core.SortSymbols(idx.symbols)
	core.SortOccurrences(idx.occurrences)
	return idx, nil
}

func goDebtMarkers(p string, src []byte, fset *token.FileSet, file *ast.File) []core.DebtMarker {
	var out []core.DebtMarker
	for _, group := range file.Comments {
		for _, comment := range group.List {
			start := fset.Position(comment.Pos()).Offset
			end := fset.Position(comment.End()).Offset
			out = append(out, core.FindDebtMarkersInRange(p, src, start, end)...)
		}
	}
	return out
}

func exportIndex(idx *index) *Index {
	return &Index{
		Documents:   append([]Document(nil), idx.documents...),
		Packages:    append([]PackageInfo(nil), idx.packages...),
		Symbols:     append([]Symbol(nil), idx.symbols...),
		Occurrences: append([]Occurrence(nil), idx.occurrences...),
		Edges:       append([]Edge(nil), idx.edges...),
		Imports:     append([]ImportEdge(nil), idx.imports...),
		Diagnostics: append([]Diagnostic(nil), idx.diagnostics...),
		ByID:        idx.byID,
		ByName:      idx.byName,
		UnitFiles:   idx.unitFiles,
		FileUnits:   idx.fileUnits,
		FileLOC:     idx.fileLOC,
	}
}

func indexDecls(idx *index, pf parsedFile) {
	for _, decl := range pf.file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			sym := funcSymbol(pf, d)
			addSymbol(idx, sym)
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					sym := typeSymbol(pf, d, s)
					addSymbol(idx, sym)
				case *ast.ValueSpec:
					for _, name := range s.Names {
						kind := SymbolVar
						if d.Tok == token.CONST {
							kind = SymbolConst
						}
						sym := valueSymbol(pf, d, s, name, kind)
						addSymbol(idx, sym)
					}
				}
			}
		}
	}
}

func addSymbol(idx *index, sym Symbol) {
	idx.symbols = append(idx.symbols, sym)
	idx.byID[sym.ID] = sym
	idx.byName[sym.Name] = append(idx.byName[sym.Name], sym)
	idx.byName[sym.QualifiedName] = append(idx.byName[sym.QualifiedName], sym)
	idx.occurrences = append(idx.occurrences, Occurrence{
		SymbolID: sym.ID,
		Kind:     OccurrenceDeclaration,
		Name:     sym.Name,
		Location: Location{URI: sym.Location.URI, Range: sym.SelectionRange},
	})
	for _, child := range sym.Children {
		idx.symbols = append(idx.symbols, child)
		idx.byID[child.ID] = child
		idx.byName[child.Name] = append(idx.byName[child.Name], child)
		idx.byName[child.QualifiedName] = append(idx.byName[child.QualifiedName], child)
		idx.occurrences = append(idx.occurrences, Occurrence{
			SymbolID: child.ID,
			Kind:     OccurrenceDeclaration,
			Name:     child.Name,
			Location: Location{URI: child.Location.URI, Range: child.SelectionRange},
		})
		idx.edges = append(idx.edges, Edge{Kind: EdgeContains, From: string(sym.ID), To: string(child.ID), Location: child.Location})
	}
}

func funcSymbol(pf parsedFile, fn *ast.FuncDecl) Symbol {
	kind := SymbolFunction
	qname := fn.Name.Name
	container := ""
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		kind = SymbolMethod
		recv := receiverName(fn.Recv.List[0].Type)
		container = strings.TrimPrefix(recv, "*")
		qname = recv + "." + fn.Name.Name
	}
	sym := Symbol{
		ID:             symbolID(pf.path, kind, qname, pf.fset.Position(fn.Pos()).Offset),
		Language:       Go,
		Kind:           kind,
		Name:           fn.Name.Name,
		QualifiedName:  qname,
		ContainerName:  container,
		UnitID:         pf.unit,
		Location:       Location{URI: pf.path, Range: rangeOf(pf.fset, fn.Pos(), fn.End())},
		SelectionRange: rangeOf(pf.fset, fn.Name.Pos(), fn.Name.End()),
		Signature:      signatureForFunc(pf, fn),
		Doc:            commentText(fn.Doc),
		Tags:           map[string]string{},
		Backend:        goBackend(),
	}
	if fn.Body != nil {
		sym.BodyRange = rangeOf(pf.fset, fn.Body.Pos(), fn.Body.End())
	}
	return sym
}

func typeSymbol(pf parsedFile, gen *ast.GenDecl, ts *ast.TypeSpec) Symbol {
	kind := SymbolType
	switch ts.Type.(type) {
	case *ast.StructType:
		kind = SymbolStruct
	case *ast.InterfaceType:
		kind = SymbolInterface
	}
	sym := Symbol{
		ID:             symbolID(pf.path, kind, ts.Name.Name, pf.fset.Position(ts.Pos()).Offset),
		Language:       Go,
		Kind:           kind,
		Name:           ts.Name.Name,
		QualifiedName:  ts.Name.Name,
		UnitID:         pf.unit,
		Location:       Location{URI: pf.path, Range: declRange(pf.fset, gen, ts)},
		SelectionRange: rangeOf(pf.fset, ts.Name.Pos(), ts.Name.End()),
		Signature:      sourceSlice(pf.src, pf.fset, ts.Pos(), ts.End()),
		Doc:            commentText(firstComment(ts.Doc, gen.Doc)),
		Tags:           map[string]string{},
		Backend:        goBackend(),
	}
	switch t := ts.Type.(type) {
	case *ast.StructType:
		for _, field := range t.Fields.List {
			for _, name := range field.Names {
				child := fieldSymbol(pf, sym, name, field)
				sym.Children = append(sym.Children, child)
			}
		}
	case *ast.InterfaceType:
		for _, method := range t.Methods.List {
			for _, name := range method.Names {
				child := interfaceMethodSymbol(pf, sym, name, method)
				sym.Children = append(sym.Children, child)
			}
		}
	}
	return sym
}

func valueSymbol(pf parsedFile, gen *ast.GenDecl, spec *ast.ValueSpec, name *ast.Ident, kind SymbolKind) Symbol {
	return Symbol{
		ID:             symbolID(pf.path, kind, name.Name, pf.fset.Position(name.Pos()).Offset),
		Language:       Go,
		Kind:           kind,
		Name:           name.Name,
		QualifiedName:  name.Name,
		UnitID:         pf.unit,
		Location:       Location{URI: pf.path, Range: declRange(pf.fset, gen, spec)},
		SelectionRange: rangeOf(pf.fset, name.Pos(), name.End()),
		Signature:      sourceSlice(pf.src, pf.fset, spec.Pos(), spec.End()),
		Doc:            commentText(firstComment(spec.Doc, gen.Doc)),
		Tags:           map[string]string{},
		Backend:        goBackend(),
	}
}

func fieldSymbol(pf parsedFile, parent Symbol, name *ast.Ident, field *ast.Field) Symbol {
	tags := map[string]string{}
	if field.Tag != nil {
		tags["go.tag"] = strings.Trim(field.Tag.Value, "`")
	}
	return Symbol{
		ID:             symbolID(pf.path, SymbolField, parent.Name+"."+name.Name, pf.fset.Position(name.Pos()).Offset),
		Language:       Go,
		Kind:           SymbolField,
		Name:           name.Name,
		QualifiedName:  parent.Name + "." + name.Name,
		ContainerID:    parent.ID,
		ContainerName:  parent.Name,
		UnitID:         pf.unit,
		Location:       Location{URI: pf.path, Range: rangeOf(pf.fset, field.Pos(), field.End())},
		SelectionRange: rangeOf(pf.fset, name.Pos(), name.End()),
		Signature:      sourceSlice(pf.src, pf.fset, field.Pos(), field.End()),
		Doc:            commentText(field.Doc),
		Tags:           tags,
		Backend:        goBackend(),
	}
}

func interfaceMethodSymbol(pf parsedFile, parent Symbol, name *ast.Ident, field *ast.Field) Symbol {
	return Symbol{
		ID:             symbolID(pf.path, SymbolMethod, parent.Name+"."+name.Name, pf.fset.Position(name.Pos()).Offset),
		Language:       Go,
		Kind:           SymbolMethod,
		Name:           name.Name,
		QualifiedName:  parent.Name + "." + name.Name,
		ContainerID:    parent.ID,
		ContainerName:  parent.Name,
		UnitID:         pf.unit,
		Location:       Location{URI: pf.path, Range: rangeOf(pf.fset, field.Pos(), field.End())},
		SelectionRange: rangeOf(pf.fset, name.Pos(), name.End()),
		Signature:      sourceSlice(pf.src, pf.fset, field.Pos(), field.End()),
		Doc:            commentText(field.Doc),
		Tags:           map[string]string{},
		Backend:        goBackend(),
	}
}

func indexImports(idx *index, pf parsedFile) {
	for _, imp := range pf.file.Imports {
		importPath := strings.Trim(imp.Path.Value, "\"")
		alias := ""
		if imp.Name != nil {
			alias = imp.Name.Name
		}
		loc := Location{URI: pf.path, Range: rangeOf(pf.fset, imp.Pos(), imp.End())}
		idx.imports = append(idx.imports, ImportEdge{
			FromUnit: pf.unit,
			FromPath: pf.path,
			Import:   importPath,
			Alias:    alias,
			Location: loc,
		})
		idx.edges = append(idx.edges, Edge{Kind: EdgeImports, From: pf.unit, To: importPath, Location: loc, Weight: 1})
		idx.occurrences = append(idx.occurrences, Occurrence{SymbolID: importOccurrenceID(pf, imp, importPath), Kind: OccurrenceImport, Name: importOccurrenceName(importPath, alias), Location: loc, Preview: sourceLine(pf.src, loc.Range.Start.Offset)})
	}
}

func importOccurrenceID(pf parsedFile, imp *ast.ImportSpec, importPath string) SymbolID {
	return symbolID(pf.path, SymbolImport, importPath, pf.fset.Position(imp.Pos()).Offset)
}

func importOccurrenceName(importPath, alias string) string {
	name := alias
	if name == "" {
		name = path.Base(importPath)
	}
	return name
}

func indexUses(idx *index, pf parsedFile) {
	classified := classifiedOccurrences(pf)
	for _, decl := range pf.file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		caller := findCallable(idx, pf, fn)
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if x, ok := n.(*ast.CallExpr); ok {
				if callee := callTarget(idx, pf.unit, x.Fun); callee.ID != "" && caller.ID != "" {
					loc := Location{URI: pf.path, Range: callNameRange(pf.fset, x.Fun)}
					idx.edges = append(idx.edges, Edge{Kind: EdgeCalls, From: string(caller.ID), To: string(callee.ID), Location: loc, Weight: 1})
					idx.occurrences = append(idx.occurrences, Occurrence{SymbolID: callee.ID, Kind: OccurrenceCall, Name: callee.Name, Location: loc, Preview: sourceLine(pf.src, loc.Range.Start.Offset)})
				}
			}
			return true
		})
	}
	ast.Inspect(pf.file, func(n ast.Node) bool {
		x, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		if classified.skip[x] {
			return true
		}
		kind := classified.kind[x]
		if x.Obj != nil && x.Obj.Pos() == x.Pos() && kind != OccurrenceWrite {
			return true
		}
		sym := symbolForIdent(idx, pf, x, kind == OccurrenceWrite)
		if sym.ID == "" {
			return true
		}
		loc := Location{URI: pf.path, Range: rangeOf(pf.fset, x.Pos(), x.End())}
		if kind == "" {
			kind = OccurrenceRead
		}
		idx.occurrences = append(idx.occurrences, Occurrence{SymbolID: sym.ID, Kind: kind, Name: x.Name, Location: loc, Preview: sourceLine(pf.src, loc.Range.Start.Offset)})
		idx.edges = append(idx.edges, Edge{Kind: EdgeReferences, From: pf.path, To: string(sym.ID), Location: loc, Weight: 1})
		return true
	})
	indexDocOccurrences(idx, pf)
}

type occurrenceClassification struct {
	kind map[*ast.Ident]OccurrenceKind
	skip map[*ast.Ident]bool
}

func classifiedOccurrences(pf parsedFile) occurrenceClassification {
	c := occurrenceClassification{kind: map[*ast.Ident]OccurrenceKind{}, skip: map[*ast.Ident]bool{}}
	ast.Inspect(pf.file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			for _, expr := range x.Lhs {
				markWriteExpr(c, expr)
			}
		case *ast.IncDecStmt:
			markWriteExpr(c, x.X)
		case *ast.RangeStmt:
			if x.Key != nil {
				markWriteExpr(c, x.Key)
			}
			if x.Value != nil {
				markWriteExpr(c, x.Value)
			}
		case *ast.ValueSpec:
			if len(x.Values) > 0 {
				for _, name := range x.Names {
					if name.Name != "_" {
						c.kind[name] = OccurrenceWrite
					}
				}
			}
		case *ast.CallExpr:
			markCallExpr(c, x.Fun)
		case *ast.KeyValueExpr:
			if id, ok := x.Key.(*ast.Ident); ok {
				c.skip[id] = true
			}
		case *ast.SelectorExpr:
			c.skip[x.Sel] = selectorNameShouldSkip(c, x.Sel)
		}
		return true
	})
	return c
}

func markWriteExpr(c occurrenceClassification, expr ast.Expr) {
	switch x := expr.(type) {
	case *ast.Ident:
		if x.Name != "_" {
			c.kind[x] = OccurrenceWrite
		}
	case *ast.SelectorExpr:
		c.kind[x.Sel] = OccurrenceWrite
	case *ast.IndexExpr:
		markWriteExpr(c, x.X)
	case *ast.IndexListExpr:
		markWriteExpr(c, x.X)
	case *ast.StarExpr:
		markWriteExpr(c, x.X)
	case *ast.ParenExpr:
		markWriteExpr(c, x.X)
	}
}

func markCallExpr(c occurrenceClassification, expr ast.Expr) {
	switch x := expr.(type) {
	case *ast.Ident:
		c.skip[x] = true
	case *ast.SelectorExpr:
		c.skip[x.Sel] = true
	}
}

func selectorNameShouldSkip(c occurrenceClassification, id *ast.Ident) bool {
	if _, ok := c.kind[id]; ok {
		return false
	}
	return c.skip[id]
}

func symbolForIdent(idx *index, pf parsedFile, id *ast.Ident, includeDeclaration bool) Symbol {
	offset := pf.fset.Position(id.Pos()).Offset
	for _, sym := range idx.byName[id.Name] {
		if sym.UnitID == pf.unit && (includeDeclaration || sym.SelectionRange.Start.Offset != offset) {
			return sym
		}
	}
	return Symbol{}
}

func indexDocOccurrences(idx *index, pf parsedFile) {
	for _, decl := range pf.file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Doc == nil {
				continue
			}
			sym := findCallable(idx, pf, d)
			addDocOccurrence(idx, pf, sym, d.Doc)
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					sym := firstUnitSymbol(idx.byName[s.Name.Name], pf.unit, "", "")
					addDocOccurrence(idx, pf, sym, firstComment(s.Doc, d.Doc))
				case *ast.ValueSpec:
					for _, name := range s.Names {
						sym := firstUnitSymbol(idx.byName[name.Name], pf.unit, "", "")
						addDocOccurrence(idx, pf, sym, firstComment(s.Doc, d.Doc))
					}
				}
			}
		}
	}
}

func addDocOccurrence(idx *index, pf parsedFile, sym Symbol, group *ast.CommentGroup) {
	if sym.ID == "" || group == nil {
		return
	}
	loc := Location{URI: pf.path, Range: rangeOf(pf.fset, group.Pos(), group.End())}
	idx.occurrences = append(idx.occurrences, Occurrence{
		SymbolID: sym.ID,
		Kind:     OccurrenceDoc,
		Name:     sym.Name,
		Location: loc,
		Preview:  sourceLine(pf.src, loc.Range.Start.Offset),
	})
}

func indexImplementations(idx *index) {
	interfaces := map[string]Symbol{}
	methodSets := map[string]map[string]bool{}
	for _, sym := range idx.symbols {
		if sym.Kind == SymbolInterface {
			interfaces[string(sym.ID)] = sym
			set := map[string]bool{}
			for _, child := range sym.Children {
				set[child.Name] = true
			}
			methodSets[string(sym.ID)] = set
		}
	}
	for _, typ := range idx.symbols {
		if typ.Kind != SymbolStruct {
			continue
		}
		methods := map[string]bool{}
		for _, sym := range idx.symbols {
			if sym.Kind == SymbolMethod && sym.UnitID == typ.UnitID && strings.TrimPrefix(sym.ContainerName, "*") == typ.Name {
				methods[sym.Name] = true
			}
		}
		for id, iface := range interfaces {
			needed := methodSets[id]
			if len(needed) == 0 {
				continue
			}
			ok := true
			for name := range needed {
				if !methods[name] {
					ok = false
					break
				}
			}
			if ok {
				idx.edges = append(idx.edges, Edge{
					Kind:     EdgeImplements,
					From:     string(typ.ID),
					To:       string(iface.ID),
					Location: typ.Location,
					Weight:   1,
					Evidence: []Evidence{{Kind: "ast_method_set", Message: "struct has methods matching interface names"}},
				})
			}
		}
	}
}

func indexPackages(idx *index) {
	idx.packages = nil
	for unit, files := range idx.unitFiles {
		sort.Strings(files)
		name := unit
		dir := packageDir(unit)
		if i := strings.IndexByte(unit, '#'); i >= 0 {
			name = unit[i+1:]
		}
		idx.packages = append(idx.packages, PackageInfo{
			ID:    unit,
			Name:  name,
			Dir:   dir,
			Files: append([]string(nil), files...),
		})
	}
	sort.Slice(idx.packages, func(i, j int) bool {
		return idx.packages[i].ID < idx.packages[j].ID
	})
}

func findCallable(idx *index, pf parsedFile, fn *ast.FuncDecl) Symbol {
	qname := fn.Name.Name
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		qname = receiverName(fn.Recv.List[0].Type) + "." + fn.Name.Name
	}
	for _, sym := range idx.byName[qname] {
		if sym.UnitID == pf.unit && sym.Location.URI == pf.path {
			return sym
		}
	}
	return Symbol{}
}

func callTarget(idx *index, unit string, expr ast.Expr) Symbol {
	switch x := expr.(type) {
	case *ast.Ident:
		return firstUnitSymbol(idx.byName[x.Name], unit, SymbolFunction, "")
	case *ast.SelectorExpr:
		for _, sym := range idx.byName[x.Sel.Name] {
			if sym.UnitID == unit && (sym.Kind == SymbolMethod || sym.Kind == SymbolFunction) {
				return sym
			}
		}
	}
	return Symbol{}
}

func callNameRange(fset *token.FileSet, expr ast.Expr) Range {
	switch x := expr.(type) {
	case *ast.Ident:
		return rangeOf(fset, x.Pos(), x.End())
	case *ast.SelectorExpr:
		return rangeOf(fset, x.Sel.Pos(), x.Sel.End())
	default:
		return rangeOf(fset, expr.Pos(), expr.End())
	}
}

func firstUnitSymbol(symbols []Symbol, unit string, kind SymbolKind, qname string) Symbol {
	for _, sym := range symbols {
		if sym.UnitID == unit && (kind == "" || sym.Kind == kind) && (qname == "" || sym.QualifiedName == qname) {
			return sym
		}
	}
	return Symbol{}
}

func computeMetrics(idx *index) []UnitMetrics {
	metrics := map[string]*UnitMetrics{}
	for unit, files := range idx.unitFiles {
		metrics[unit] = &UnitMetrics{UnitID: unit, FileCount: len(files)}
		for _, p := range files {
			metrics[unit].LOC += idx.fileLOC[p]
		}
	}
	for _, imp := range idx.imports {
		m := ensureUnitMetric(metrics, imp.FromUnit)
		m.DirectFanOut++
		for unit := range metrics {
			if strings.HasSuffix(imp.Import, packageDir(unit)) {
				metrics[unit].DirectFanIn++
			}
		}
	}
	for _, sym := range idx.symbols {
		m := ensureUnitMetric(metrics, sym.UnitID)
		if isExported(sym.Name) {
			m.PublicSymbolCount++
		}
		if sym.Kind == SymbolInterface {
			m.InterfaceCount++
		}
	}
	for _, edge := range idx.edges {
		if edge.Kind == EdgeCalls {
			if from, ok := idx.byID[SymbolID(edge.From)]; ok {
				ensureUnitMetric(metrics, from.UnitID).CallFanOut++
				ensureUnitMetric(metrics, from.UnitID).SymbolFanOut++
			}
			if to, ok := idx.byID[SymbolID(edge.To)]; ok {
				ensureUnitMetric(metrics, to.UnitID).CallFanIn++
				ensureUnitMetric(metrics, to.UnitID).SymbolFanIn++
			}
		}
		if edge.Kind == EdgeReferences {
			fromUnit := idx.fileUnits[edge.From]
			if fromUnit != "" {
				ensureUnitMetric(metrics, fromUnit).SymbolFanOut++
			}
			if to, ok := idx.byID[SymbolID(edge.To)]; ok {
				ensureUnitMetric(metrics, to.UnitID).SymbolFanIn++
			}
		}
		if edge.Kind == EdgeImplements {
			if from, ok := idx.byID[SymbolID(edge.From)]; ok {
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
	sortUnitMetrics(out)
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

func unitID(filePath, pkg string) string {
	dir := path.Dir(filePath)
	if dir == "." {
		return pkg
	}
	return dir + "#" + pkg
}

func packageDir(unit string) string {
	if i := strings.IndexByte(unit, '#'); i >= 0 {
		return unit[:i]
	}
	return unit
}

func symbolID(filePath string, kind SymbolKind, qname string, offset int) SymbolID {
	return SymbolID(fmt.Sprintf("%s:%s:%s:%d", filePath, kind, qname, offset))
}

func rangeOf(fset *token.FileSet, start, end token.Pos) Range {
	sp := fset.Position(start)
	ep := fset.Position(end)
	return Range{
		Start: Position{Line: sp.Line, Column: sp.Column, Offset: sp.Offset},
		End:   Position{Line: ep.Line, Column: ep.Column, Offset: ep.Offset},
	}
}

func declRange(fset *token.FileSet, gen *ast.GenDecl, spec ast.Spec) Range {
	if len(gen.Specs) == 1 {
		return rangeOf(fset, gen.Pos(), gen.End())
	}
	return rangeOf(fset, spec.Pos(), spec.End())
}

func sourceSlice(src []byte, fset *token.FileSet, start, end token.Pos) string {
	r := rangeOf(fset, start, end)
	if r.Start.Offset < 0 || r.End.Offset > len(src) || r.Start.Offset > r.End.Offset {
		return ""
	}
	return string(src[r.Start.Offset:r.End.Offset])
}

func sourceLine(src []byte, offset int) string {
	if offset < 0 || offset > len(src) {
		return ""
	}
	start := offset
	for start > 0 && src[start-1] != '\n' {
		start--
	}
	end := offset
	for end < len(src) && src[end] != '\n' {
		end++
	}
	return strings.TrimSpace(string(src[start:end]))
}

func signatureForFunc(pf parsedFile, fn *ast.FuncDecl) string {
	if fn.Body == nil {
		return sourceSlice(pf.src, pf.fset, fn.Pos(), fn.End())
	}
	return strings.TrimSpace(sourceSlice(pf.src, pf.fset, fn.Pos(), fn.Body.Lbrace))
}

func receiverName(expr ast.Expr) string {
	switch x := expr.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.StarExpr:
		return "*" + receiverName(x.X)
	case *ast.IndexExpr:
		return receiverName(x.X)
	case *ast.IndexListExpr:
		return receiverName(x.X)
	default:
		return ""
	}
}

func commentText(group *ast.CommentGroup) string {
	if group == nil {
		return ""
	}
	return strings.TrimSpace(group.Text())
}

func firstComment(groups ...*ast.CommentGroup) *ast.CommentGroup {
	for _, group := range groups {
		if group != nil {
			return group
		}
	}
	return nil
}

func goBackend() BackendInfo {
	return BackendInfo{
		Language:       Go,
		Name:           "goast",
		ResolutionMode: "ast",
		Complete:       false,
		Diagnostics:    []Diagnostic{{Severity: "info", Message: "Go backend uses AST-only best-effort resolution"}},
	}
}

func countLines(src []byte) int {
	if len(src) == 0 {
		return 0
	}
	n := 1
	for _, b := range src {
		if b == '\n' {
			n++
		}
	}
	return n
}

func isExported(name string) bool {
	if name == "" {
		return false
	}
	r := []rune(name)[0]
	return r >= 'A' && r <= 'Z'
}

func sortUnitMetrics(metrics []UnitMetrics) {
	sort.Slice(metrics, func(i, j int) bool {
		return metrics[i].UnitID < metrics[j].UnitID
	})
}

func parseTagLiteral(value string) map[string]string {
	tags := map[string]string{}
	value = strings.Trim(value, "`")
	for value != "" {
		value = strings.TrimLeft(value, " \t")
		i := strings.Index(value, ":")
		if i <= 0 {
			break
		}
		key := value[:i]
		value = value[i+1:]
		if !strings.HasPrefix(value, "\"") {
			break
		}
		quoted, rest, ok := cutQuoted(value)
		if !ok {
			break
		}
		unquoted, err := strconv.Unquote(quoted)
		if err == nil {
			tags[key] = unquoted
		}
		value = rest
	}
	return tags
}

func formatTagLiteral(tags map[string]string) string {
	if len(tags) == 0 {
		return ""
	}
	keys := make([]string, 0, len(tags))
	for key := range tags {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+":"+strconv.Quote(tags[key]))
	}
	return "`" + strings.Join(parts, " ") + "`"
}

func cutQuoted(s string) (quoted, rest string, ok bool) {
	escaped := false
	for i := 1; i < len(s); i++ {
		c := s[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' {
			escaped = true
			continue
		}
		if c == '"' {
			return s[:i+1], s[i+1:], true
		}
	}
	return "", "", false
}
