package editor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/codewandler/editor/internal/core"
	"github.com/codewandler/editor/internal/lang/goast"
)

type Option func(*Editor) error

type Editor struct {
	root      string
	fsys      fs.FS
	source    Source
	languages []LanguageID
	backends  map[LanguageID]Backend

	mu      sync.RWMutex
	overlay map[string][]byte
}

func New(root string, opts ...Option) (*Editor, error) {
	ed := &Editor{
		root:      core.CleanPath(root),
		languages: []LanguageID{Go},
		backends:  map[LanguageID]Backend{},
		overlay:   map[string][]byte{},
	}
	for _, opt := range opts {
		if err := opt(ed); err != nil {
			return nil, err
		}
	}
	if ed.fsys == nil && ed.source == nil {
		return nil, errors.New("editor: WithFS or WithSource is required")
	}
	if _, ok := ed.backends[Go]; !ok {
		ed.backends[Go] = goast.New()
	}
	for _, lang := range ed.languages {
		if _, ok := ed.backends[lang]; !ok {
			return nil, fmt.Errorf("editor: no backend registered for language %q", lang)
		}
	}
	return ed, nil
}

func WithFS(fsys fs.FS) Option {
	return func(ed *Editor) error {
		if fsys == nil {
			return errors.New("editor: nil fs.FS")
		}
		ed.fsys = fsys
		return nil
	}
}

func WithSource(source Source) Option {
	return func(ed *Editor) error {
		if source == nil {
			return errors.New("editor: nil source")
		}
		ed.source = source
		return nil
	}
}

func WithLanguage(lang LanguageID) Option {
	return func(ed *Editor) error {
		if lang == "" {
			return errors.New("editor: empty language")
		}
		ed.languages = []LanguageID{lang}
		return nil
	}
}

func WithBackend(backend Backend) Option {
	return func(ed *Editor) error {
		if backend == nil {
			return errors.New("editor: nil backend")
		}
		spec := backend.Spec()
		if spec.Language == "" {
			return errors.New("editor: backend has empty language")
		}
		ed.backends[spec.Language] = backend
		return nil
	}
}

func (e *Editor) Outline(ctx context.Context, scope Scope) (Outline, error) {
	idx, err := e.buildIndex(ctx, scope, nil)
	if err != nil {
		return Outline{}, err
	}
	return Outline{Documents: idx.Documents, Symbols: idx.Symbols, Diagnostics: idx.Diagnostics}, nil
}

func (e *Editor) Packages(ctx context.Context, scope Scope) (PackageResult, error) {
	idx, err := e.buildIndex(ctx, scope, nil)
	if err != nil {
		return PackageResult{}, err
	}
	return PackageResult{
		Packages:    append([]PackageInfo(nil), idx.Packages...),
		Diagnostics: idx.Diagnostics,
		Indexed:     true,
		Fresh:       true,
	}, nil
}

func (e *Editor) FindSymbols(ctx context.Context, sel SymbolSelector) ([]Symbol, error) {
	idx, err := e.buildIndex(ctx, core.SelectorScope(sel), nil)
	if err != nil {
		return nil, err
	}
	return core.FilterSymbols(idx.Symbols, sel), nil
}

func (e *Editor) Definition(ctx context.Context, sel SymbolSelector) (ResolveResult, error) {
	idx, err := e.buildIndex(ctx, core.SelectorScope(sel), nil)
	if err != nil {
		return ResolveResult{}, err
	}
	matches := core.FilterSymbols(idx.Symbols, sel)
	return ResolveResult{Matches: matches, Ambiguous: len(matches) > 1, Diagnostics: idx.Diagnostics}, nil
}

func (e *Editor) DefinitionAt(ctx context.Context, pos PositionSelector) (ResolveResult, error) {
	nav, err := e.Navigate(ctx, pos, NavigationOptions{})
	if err != nil {
		return ResolveResult{}, err
	}
	return ResolveResult{Matches: nav.Symbols, Ambiguous: len(nav.Symbols) > 1, Diagnostics: nav.Diagnostics}, nil
}

func (e *Editor) Navigate(ctx context.Context, pos PositionSelector, opts NavigationOptions) (NavigationResult, error) {
	scope := opts.Scope
	if scope.Path == "" {
		scope.Path = navigationScopePath(pos.Path)
	}
	idx, err := e.buildIndex(ctx, scope, nil)
	if err != nil {
		return NavigationResult{}, err
	}
	offset := -1
	if pos.Offset != nil {
		offset = *pos.Offset
	} else {
		b, err := e.readFileWithOverlay(ctx, core.CleanPath(pos.Path), nil)
		if err != nil {
			return NavigationResult{}, err
		}
		offset = core.LineColumnOffset(b, pos.Line, pos.Column)
	}
	target, symbols, diagnostics := e.resolvePositionTarget(idx, pos.Path, offset, opts.FallbackEnclosing)
	if opts.MaxResults > 0 && len(symbols) > opts.MaxResults {
		symbols = symbols[:opts.MaxResults]
	}
	locations := make([]Location, 0, len(symbols))
	for _, sym := range symbols {
		locations = append(locations, sym.Location)
	}
	diagnostics = append(idx.Diagnostics, diagnostics...)
	return NavigationResult{
		Target:         target,
		Symbols:        symbols,
		Locations:      locations,
		Diagnostics:    diagnostics,
		ResolutionMode: e.resolutionMode(scope),
		Complete:       false,
		Warnings:       []string{"AST-only resolution: no type checking, external dependency resolution, build-tag/cgo semantics, interface dispatch, or function-value dispatch."},
		Indexed:        true,
		Fresh:          true,
	}, nil
}

func (e *Editor) SymbolInfo(ctx context.Context, pos PositionSelector, opts NavigationOptions) (NavigationResult, error) {
	opts.FallbackEnclosing = true
	return e.Navigate(ctx, pos, opts)
}

func (e *Editor) References(ctx context.Context, sel SymbolSelector) ([]Occurrence, error) {
	idx, err := e.buildIndex(ctx, core.SelectorScope(sel), nil)
	if err != nil {
		return nil, err
	}
	targets := core.FilterSymbols(idx.Symbols, sel)
	targetIDs := map[SymbolID]bool{}
	for _, sym := range targets {
		targetIDs[sym.ID] = true
	}
	var out []Occurrence
	for _, occ := range idx.Occurrences {
		if targetIDs[occ.SymbolID] {
			out = append(out, occ)
		}
	}
	core.SortOccurrences(out)
	return out, nil
}

func (e *Editor) ReferencesAt(ctx context.Context, pos PositionSelector, opts ReferenceOptions) ([]Occurrence, error) {
	navOpts := NavigationOptions{Scope: opts.Scope}
	nav, err := e.Navigate(ctx, pos, navOpts)
	if err != nil {
		return nil, err
	}
	if len(nav.Symbols) == 0 {
		return nil, nil
	}

	referenceScope := opts.Scope
	if referenceScope.Language == "" {
		referenceScope.Language = nav.Symbols[0].Language
	}
	idx, err := e.buildIndex(ctx, referenceScope, nil)
	if err != nil {
		return nil, err
	}
	targetID := nav.Symbols[0].ID
	refs := make([]Occurrence, 0)
	for _, occ := range idx.Occurrences {
		if occ.SymbolID != targetID || !referenceInScope(idx, occ, opts.Scope) {
			continue
		}
		if !opts.IncludeDeclaration && occ.Kind == OccurrenceDeclaration {
			continue
		}
		refs = append(refs, occ)
		if opts.MaxResults > 0 && len(refs) >= opts.MaxResults {
			break
		}
	}
	return refs, nil
}

func referenceInScope(idx *core.Index, occ Occurrence, scope Scope) bool {
	uri := core.CleanPath(occ.Location.URI)
	if scope.Path != "" && !inScopePath(uri, scope.Path) {
		return false
	}
	if scope.UnitID != "" {
		if idx.FileUnits[uri] != scope.UnitID {
			return false
		}
	}
	if !scope.IncludeTests && core.HasTestPath(uri) {
		return false
	}
	return true
}

func (e *Editor) Implementations(ctx context.Context, sel SymbolSelector) ([]Implementation, error) {
	idx, err := e.buildIndex(ctx, core.SelectorScope(sel), nil)
	if err != nil {
		return nil, err
	}
	targets := core.FilterSymbols(idx.Symbols, sel)
	var out []Implementation
	for _, target := range targets {
		if target.Kind != SymbolInterface {
			continue
		}
		for _, edge := range idx.Edges {
			if edge.Kind != EdgeImplements || edge.To != string(target.ID) {
				continue
			}
			if concrete, ok := idx.ByID[SymbolID(edge.From)]; ok {
				out = append(out, Implementation{Interface: target, Concrete: concrete, Location: edge.Location, Evidence: edge.Evidence})
			}
		}
	}
	return out, nil
}

func (e *Editor) Callers(ctx context.Context, sel SymbolSelector) ([]CallEdge, error) {
	idx, err := e.buildIndex(ctx, core.SelectorScope(sel), nil)
	if err != nil {
		return nil, err
	}
	targets := core.FilterSymbols(idx.Symbols, sel)
	targetIDs := map[string]bool{}
	for _, sym := range targets {
		targetIDs[string(sym.ID)] = true
	}
	var out []CallEdge
	for _, edge := range idx.Edges {
		if edge.Kind != EdgeCalls || !targetIDs[edge.To] {
			continue
		}
		caller, ok1 := idx.ByID[SymbolID(edge.From)]
		callee, ok2 := idx.ByID[SymbolID(edge.To)]
		if ok1 && ok2 {
			out = append(out, CallEdge{CallerID: edge.From, CalleeID: edge.To, Caller: caller, Callee: callee, Name: callee.Name, Kind: string(edge.Kind), Location: edge.Location})
		}
	}
	return out, nil
}

func (e *Editor) Callees(ctx context.Context, sel SymbolSelector) ([]CallEdge, error) {
	idx, err := e.buildIndex(ctx, core.SelectorScope(sel), nil)
	if err != nil {
		return nil, err
	}
	targets := core.FilterSymbols(idx.Symbols, sel)
	targetIDs := map[string]bool{}
	for _, sym := range targets {
		targetIDs[string(sym.ID)] = true
	}
	var out []CallEdge
	for _, edge := range idx.Edges {
		if edge.Kind != EdgeCalls || !targetIDs[edge.From] {
			continue
		}
		caller, ok1 := idx.ByID[SymbolID(edge.From)]
		callee, ok2 := idx.ByID[SymbolID(edge.To)]
		if ok1 && ok2 {
			out = append(out, CallEdge{CallerID: edge.From, CalleeID: edge.To, Caller: caller, Callee: callee, Name: callee.Name, Kind: string(edge.Kind), Location: edge.Location})
		}
	}
	return out, nil
}

func (e *Editor) Imports(ctx context.Context, scope Scope) ([]ImportEdge, error) {
	idx, err := e.buildIndex(ctx, scope, nil)
	if err != nil {
		return nil, err
	}
	return append([]ImportEdge(nil), idx.Imports...), nil
}

func (e *Editor) ImportGraph(ctx context.Context, query ImportQuery) (ImportResult, error) {
	scope := query.Scope
	if scope.Path == "" {
		scope.Path = query.Path
		if scope.Path == "" && strings.Contains(query.PackageID, "#") {
			scope.Path = packagePath(query.PackageID)
		}
	}
	if query.IncludeTest != nil {
		scope.IncludeTests = *query.IncludeTest
	}
	idx, err := e.buildIndex(ctx, scope, nil)
	if err != nil {
		return ImportResult{}, err
	}
	direction := query.Direction
	if direction == "" {
		direction = ImportDirectionBoth
	}
	target := query.ImportPath
	var direct, reverse []ImportEdge
	for _, imp := range idx.Imports {
		fromMatches := true
		if query.Path != "" {
			queryPath := core.CleanPath(query.Path)
			fromPath := core.CleanPath(imp.FromPath)
			fromMatches = fromPath == queryPath || strings.HasPrefix(fromPath, queryPath+"/")
		}
		if query.PackageID != "" {
			fromMatches = fromMatches && imp.FromUnit == query.PackageID
		}
		importMatches := target == "" || imp.Import == target || strings.HasSuffix(imp.Import, "/"+target) || strings.HasSuffix(imp.Import, target)
		if (direction == ImportDirectionDirect || direction == ImportDirectionBoth) && fromMatches && importMatches {
			direct = append(direct, imp)
		}
		if (direction == ImportDirectionReverse || direction == ImportDirectionBoth) && importMatches {
			reverse = append(reverse, imp)
		}
	}
	if query.MaxResults > 0 {
		if len(direct) > query.MaxResults {
			direct = direct[:query.MaxResults]
		}
		if len(reverse) > query.MaxResults {
			reverse = reverse[:query.MaxResults]
		}
	}
	return ImportResult{
		DirectImports:    direct,
		ReverseImporters: reverse,
		TargetImportPath: target,
		Diagnostics:      idx.Diagnostics,
		ResolutionMode:   e.resolutionMode(scope),
		Complete:         false,
		Warnings:         []string{"AST-only import scan: no go list/module import path resolution."},
		Indexed:          true,
		Fresh:            true,
	}, nil
}

func (e *Editor) resolutionMode(scope Scope) string {
	mode := "ast"
	for _, backend := range e.selectedBackends(scope) {
		spec := backend.Spec()
		if spec.ResolutionMode == "hybrid" || spec.ResolutionMode == "typecheck" {
			return spec.ResolutionMode
		}
		if spec.ResolutionMode != "" {
			mode = spec.ResolutionMode
		}
	}
	return mode
}

func (e *Editor) Metrics(ctx context.Context, scope Scope) (Metrics, error) {
	idx, err := e.buildIndex(ctx, scope, nil)
	if err != nil {
		return Metrics{}, err
	}
	return Metrics{Units: computeMetrics(idx), Symbols: computeSymbolMetrics(idx), Diagnostics: idx.Diagnostics}, nil
}

func (e *Editor) ReadSymbol(ctx context.Context, sel SymbolSelector) (SourceFragment, error) {
	return e.readSymbol(ctx, sel, nil)
}

func (e *Editor) readSymbol(ctx context.Context, sel SymbolSelector, overlay map[string][]byte) (SourceFragment, error) {
	idx, err := e.buildIndex(ctx, core.SelectorScope(sel), overlay)
	if err != nil {
		return SourceFragment{}, err
	}
	matches := core.FilterSymbols(idx.Symbols, sel)
	if len(matches) == 0 {
		return SourceFragment{}, errors.New("editor: symbol not found")
	}
	if len(matches) > 1 {
		return SourceFragment{}, fmt.Errorf("editor: selector is ambiguous: %d symbols match", len(matches))
	}
	sym := matches[0]
	b, err := e.readFileWithOverlay(ctx, sym.Location.URI, overlay)
	if err != nil {
		return SourceFragment{}, err
	}
	start, end := sym.Location.Range.Start.Offset, sym.Location.Range.End.Offset
	if start < 0 || end > len(b) || start > end {
		return SourceFragment{}, errors.New("editor: invalid symbol range")
	}
	return SourceFragment{
		Symbol:   sym,
		Source:   string(b[start:end]),
		Comments: sym.Doc,
		Imports:  core.ImportsForPath(idx.Imports, sym.Location.URI),
		Hash:     contentHash(b[start:end]),
	}, nil
}

func (e *Editor) SuggestRefactorings(ctx context.Context, opts ...SuggestOption) ([]Proposal, error) {
	cfg := core.SuggestOptions{}
	for _, opt := range opts {
		opt(&cfg)
	}
	snapshot := e.snapshot(nil)
	var proposals []Proposal
	for _, backend := range e.selectedBackends(cfg.Scope) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		next, err := backend.Suggest(ctx, snapshot, cfg.Scope)
		if err != nil {
			return nil, err
		}
		proposals = append(proposals, next...)
	}
	for i := range proposals {
		proposals[i].ID = fmt.Sprintf("prop_%03d", i+1)
	}
	return proposals, nil
}

func (e *Editor) NewChangeSet() *ChangeSet {
	e.mu.RLock()
	defer e.mu.RUnlock()
	overlay := make(map[string][]byte, len(e.overlay))
	for k, v := range e.overlay {
		overlay[k] = append([]byte(nil), v...)
	}
	return &ChangeSet{
		editor:  e,
		overlay: overlay,
		changed: map[string]bool{},
	}
}

func (e *Editor) buildIndex(ctx context.Context, scope Scope, overlay map[string][]byte) (*core.Index, error) {
	out := core.NewIndex()
	snapshot := e.snapshot(overlay)
	for _, backend := range e.selectedBackends(scope) {
		idx, err := backend.Index(ctx, snapshot, scope)
		if err != nil {
			return nil, err
		}
		mergeIndex(out, idx)
	}
	core.SortSymbols(out.Symbols)
	core.SortOccurrences(out.Occurrences)
	return out, nil
}

func mergeIndex(dst, src *core.Index) {
	dst.Documents = append(dst.Documents, src.Documents...)
	dst.Packages = append(dst.Packages, src.Packages...)
	dst.Symbols = append(dst.Symbols, src.Symbols...)
	dst.Occurrences = append(dst.Occurrences, src.Occurrences...)
	dst.Edges = append(dst.Edges, src.Edges...)
	dst.Imports = append(dst.Imports, src.Imports...)
	dst.Diagnostics = append(dst.Diagnostics, src.Diagnostics...)
	for k, v := range src.ByID {
		dst.ByID[k] = v
	}
	for k, v := range src.ByName {
		dst.ByName[k] = append(dst.ByName[k], v...)
	}
	for k, v := range src.UnitFiles {
		dst.UnitFiles[k] = append(dst.UnitFiles[k], v...)
	}
	for k, v := range src.FileUnits {
		dst.FileUnits[k] = v
	}
	for k, v := range src.FileLOC {
		dst.FileLOC[k] = v
	}
}

func (e *Editor) selectedBackends(scope Scope) []Backend {
	if scope.Language != "" {
		if backend, ok := e.backends[scope.Language]; ok {
			return []Backend{backend}
		}
		return nil
	}
	var out []Backend
	for _, lang := range e.languages {
		if backend, ok := e.backends[lang]; ok {
			out = append(out, backend)
		}
	}
	return out
}

func (e *Editor) backendForPath(filePath string) (Backend, bool) {
	ext := filepath.Ext(filePath)
	for _, backend := range e.backends {
		for _, candidate := range backend.Spec().FileExtensions {
			if candidate == ext {
				return backend, true
			}
		}
	}
	return nil, false
}

func (e *Editor) backendForOperation(op Operation) (Backend, error) {
	switch op.Kind() {
	case OpRenameSymbol, OpReplaceFunction, OpAppendFunction, OpDeleteSymbol, OpReplaceComment, OpEnsureStructTag, OpRemoveStructTag:
		if backend, ok := e.backends[Go]; ok {
			return backend, nil
		}
	}
	if len(e.languages) == 1 {
		if backend, ok := e.backends[e.languages[0]]; ok {
			return backend, nil
		}
	}
	return nil, fmt.Errorf("editor: no backend supports operation %q", op.Kind())
}

func (e *Editor) readFileWithOverlay(ctx context.Context, filePath string, overlay map[string][]byte) ([]byte, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	filePath = core.CleanPath(filePath)
	if overlay != nil {
		if b, ok := overlay[filePath]; ok {
			return append([]byte(nil), b...), nil
		}
	}
	e.mu.RLock()
	if b, ok := e.overlay[filePath]; ok {
		e.mu.RUnlock()
		return append([]byte(nil), b...), nil
	}
	e.mu.RUnlock()
	if e.source != nil {
		b, err := e.source.ReadFile(ctx, filePath)
		if err != nil {
			return nil, err
		}
		return append([]byte(nil), b...), nil
	}
	b, err := fs.ReadFile(e.fsys, filePath)
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), b...), nil
}

func (e *Editor) listFiles(scope Scope, overlay map[string][]byte) ([]string, error) {
	root := core.CleanPath(firstNonEmpty(scope.Path, scope.Root, e.root))
	scopePath := core.CleanPath(scope.Path)
	seen := map[string]bool{}
	var files []string
	err := fs.WalkDir(e.fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		p = core.CleanPath(p)
		if scopePath != "" && scopePath != "." && p != scopePath && !strings.HasPrefix(p, scopePath+"/") {
			return nil
		}
		seen[p] = true
		files = append(files, p)
		return nil
	})
	if err != nil {
		return nil, err
	}
	e.mu.RLock()
	for p := range e.overlay {
		if !seen[p] && inScopePath(p, root) {
			seen[p] = true
			files = append(files, p)
		}
	}
	e.mu.RUnlock()
	for p := range overlay {
		if !seen[p] && inScopePath(p, root) {
			seen[p] = true
			files = append(files, p)
		}
	}
	sort.Strings(files)
	return files, nil
}

type editorSnapshot struct {
	editor  *Editor
	overlay map[string][]byte
}

func (e *Editor) snapshot(overlay map[string][]byte) editorSnapshot {
	return editorSnapshot{editor: e, overlay: overlay}
}

func (s editorSnapshot) ListFiles(ctx context.Context, scope Scope) ([]string, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if s.editor.source != nil {
		files, err := s.editor.source.ListFiles(ctx, scope)
		if err != nil {
			return nil, err
		}
		root := core.CleanPath(firstNonEmpty(scope.Path, scope.Root, s.editor.root))
		seen := map[string]bool{}
		for _, file := range files {
			seen[core.CleanPath(file)] = true
		}
		s.editor.mu.RLock()
		for p := range s.editor.overlay {
			if inScopePath(p, root) {
				seen[core.CleanPath(p)] = true
			}
		}
		s.editor.mu.RUnlock()
		for p := range s.overlay {
			if inScopePath(p, root) {
				seen[core.CleanPath(p)] = true
			}
		}
		out := make([]string, 0, len(seen))
		for file := range seen {
			out = append(out, file)
		}
		sort.Strings(out)
		return out, nil
	}
	return s.editor.listFiles(scope, s.overlay)
}

func (s editorSnapshot) ReadFile(ctx context.Context, filePath string) ([]byte, error) {
	return s.editor.readFileWithOverlay(ctx, filePath, s.overlay)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return "."
}

func inScopePath(filePath, root string) bool {
	filePath = core.CleanPath(filePath)
	root = core.CleanPath(root)
	return root == "." || filePath == root || strings.HasPrefix(filePath, root+"/")
}

func contentHash(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
