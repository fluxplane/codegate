package editor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/codewandler/editor/internal/core"
)

func newTestEditor(t *testing.T, files map[string]string) *Editor {
	t.Helper()
	fsys := fstest.MapFS{}
	for p, src := range files {
		fsys[p] = &fstest.MapFile{Data: []byte(src)}
	}
	ed, err := New(".", WithFS(fsys), WithLanguage(Go))
	if err != nil {
		t.Fatal(err)
	}
	return ed
}

func TestReadAPIs(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"go.mod": "module example.com/demo\n",
		"user.go": `package user

import "fmt"

type Store interface {
	Get(id string) string
}

type User struct {
	Email string
}

type MemoryStore struct{}

func (MemoryStore) Get(id string) string {
	return id
}

func CreateUser(name string, active bool) User {
	fmt.Println(name)
	return User{Email: name}
}

func Run() {
	CreateUser("a", true)
}
`,
	})

	outline, err := ed.Outline(ctx, Scope{IncludeTests: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(outline.Symbols) == 0 {
		t.Fatal("expected symbols")
	}

	users, err := ed.FindSymbols(ctx, SymbolSelector{Name: "User", Kind: SymbolStruct})
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 {
		t.Fatalf("expected one User symbol, got %d", len(users))
	}

	fragment, err := ed.ReadSymbol(ctx, SymbolSelector{Name: "CreateUser", Kind: SymbolFunction})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fragment.Source, "func CreateUser") {
		t.Fatalf("unexpected source fragment: %q", fragment.Source)
	}
	if len(fragment.Imports) != 1 || fragment.Imports[0].Import != "fmt" {
		t.Fatalf("unexpected imports: %#v", fragment.Imports)
	}

	refs, err := ed.References(ctx, SymbolSelector{Name: "CreateUser", Kind: SymbolFunction})
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) == 0 {
		t.Fatal("expected CreateUser references")
	}

	callees, err := ed.Callees(ctx, SymbolSelector{Name: "Run", Kind: SymbolFunction})
	if err != nil {
		t.Fatal(err)
	}
	if len(callees) != 1 || callees[0].Callee.Name != "CreateUser" {
		t.Fatalf("unexpected callees: %#v", callees)
	}

	impls, err := ed.Implementations(ctx, SymbolSelector{Name: "Store", Kind: SymbolInterface})
	if err != nil {
		t.Fatal(err)
	}
	if len(impls) != 1 || impls[0].Concrete.Name != "MemoryStore" {
		t.Fatalf("unexpected implementations: %#v", impls)
	}

	metrics, err := ed.Metrics(ctx, Scope{})
	if err != nil {
		t.Fatal(err)
	}
	if len(metrics.Units) != 1 || metrics.Units[0].FileCount != 1 {
		t.Fatalf("unexpected metrics: %#v", metrics)
	}
}

func TestChangeSetSemanticOps(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"user.go": `package user

type User struct {
	Email string
}

func CreateUser(name string) User {
	return User{Email: name}
}

type unused struct{}
`,
	})
	changes := ed.NewChangeSet()

	if err := changes.Apply(ctx,
		ReplaceComment{
			Target: SymbolSelector{Name: "User", Kind: SymbolStruct},
			Text:   "User is an account record.",
		},
		EnsureGoStructTag{
			Struct:  SymbolSelector{Name: "User", Kind: SymbolStruct},
			Field:   "Email",
			Key:     "json",
			Value:   "email",
			Options: []string{"omitempty"},
		},
		ReplaceFunction{
			Target: SymbolSelector{Name: "CreateUser", Kind: SymbolFunction},
			Source: `func CreateUser(name string) User {
	return User{Email: strings.TrimSpace(name)}
}`,
		},
		AppendFunction{
			Path: "user.go",
			Source: `func ValidateUser(user User) bool {
	return user.Email != ""
}`,
		},
		DeleteSymbol{Target: SymbolSelector{Name: "unused", Kind: SymbolStruct}},
	); err != nil {
		t.Fatal(err)
	}

	diff, err := changes.Diff(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"// User is an account record.",
		"`json:\"email,omitempty\"`",
		"strings.TrimSpace",
		"ValidateUser",
	} {
		if !strings.Contains(diff, want) {
			t.Fatalf("diff missing %q:\n%s", want, diff)
		}
	}
	files, err := changes.Files(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one changed file, got %d", len(files))
	}
	after := string(files[0].After)
	if strings.Contains(after, "type unused struct{}") {
		t.Fatalf("changed file still contains deleted symbol:\n%s", after)
	}

	if err := changes.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	fragment, err := ed.ReadSymbol(ctx, SymbolSelector{Name: "ValidateUser", Kind: SymbolFunction})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fragment.Source, "ValidateUser") {
		t.Fatalf("unexpected committed fragment: %q", fragment.Source)
	}
}

func TestRemoveStructTag(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"user.go": `package user

type User struct {
	Email string ` + "`json:\"email\" db:\"email\"`" + `
}
`,
	})
	changes := ed.NewChangeSet()
	if err := changes.Apply(ctx, RemoveGoStructTag{
		Struct: SymbolSelector{Name: "User", Kind: SymbolStruct},
		Field:  "Email",
		Key:    "json",
	}); err != nil {
		t.Fatal(err)
	}
	files, err := changes.Files(ctx)
	if err != nil {
		t.Fatal(err)
	}
	after := string(files[0].After)
	if strings.Contains(after, "json:") || !strings.Contains(after, "db:\"email\"") {
		t.Fatalf("unexpected changed file:\n%s", after)
	}
}

func TestSuggestRefactorings(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"demo.go": `package demo

func Render(name string, compact bool, a int, b int, c int) {}

func unusedHelper() {}
`,
	})
	proposals, err := ed.SuggestRefactorings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[RefactorKind]bool{}
	for _, proposal := range proposals {
		kinds[proposal.Kind] = true
	}
	if !kinds[RefactorDeleteSymbol] {
		t.Fatalf("expected delete-symbol proposal: %#v", proposals)
	}
	if !kinds[RefactorIntroduceConfig] {
		t.Fatalf("expected parameter proposal: %#v", proposals)
	}
	if !kinds[RefactorReplaceFlagArgument] {
		t.Fatalf("expected bool flag proposal: %#v", proposals)
	}
}

func TestGoParserParityAPIs(t *testing.T) {
	ctx := context.Background()
	files := map[string]string{
		"store/a.go": `package store

import "example.com/demo/lib"

type Store interface {
	Get(id string) string
}

type MemoryStore struct{}

func (MemoryStore) Get(id string) string {
	return lib.Normalize(id)
}

func Run() {
	CreateUser("x")
}
`,
		"store/b.go": `package store

func CreateUser(name string) string {
	return name
}
`,
		"consumer/use.go": `package consumer

import "example.com/demo/store"

func Use() {
	_ = store.MemoryStore{}
}
`,
	}
	ed := newTestEditor(t, files)

	packages, err := ed.Packages(ctx, Scope{})
	if err != nil {
		t.Fatal(err)
	}
	if len(packages.Packages) != 2 {
		t.Fatalf("expected two Go packages, got %#v", packages.Packages)
	}

	callOffset := offsetOf(t, files["store/a.go"], `CreateUser("x")`)
	nav, err := ed.Navigate(ctx, PositionSelector{Path: "store/a.go", Offset: &callOffset}, NavigationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(nav.Symbols) != 1 || nav.Symbols[0].Name != "CreateUser" || nav.Symbols[0].Location.URI != "store/b.go" {
		t.Fatalf("unexpected navigation result: %#v", nav)
	}
	if nav.ResolutionMode != "ast" || nav.Complete {
		t.Fatalf("expected incomplete AST navigation metadata: %#v", nav)
	}

	refs, err := ed.ReferencesAt(ctx, PositionSelector{Path: "store/a.go", Offset: &callOffset}, ReferenceOptions{IncludeDeclaration: false})
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) == 0 {
		t.Fatal("expected position references for CreateUser")
	}
	for _, ref := range refs {
		if ref.Kind == OccurrenceDeclaration {
			t.Fatalf("declaration should have been excluded: %#v", refs)
		}
	}

	direct, err := ed.ImportGraph(ctx, ImportQuery{Path: "store/a.go", Direction: ImportDirectionDirect})
	if err != nil {
		t.Fatal(err)
	}
	if len(direct.DirectImports) != 1 || direct.DirectImports[0].Import != "example.com/demo/lib" {
		t.Fatalf("unexpected direct imports: %#v", direct.DirectImports)
	}

	reverse, err := ed.ImportGraph(ctx, ImportQuery{ImportPath: "example.com/demo/store", Direction: ImportDirectionReverse})
	if err != nil {
		t.Fatal(err)
	}
	if len(reverse.ReverseImporters) != 1 || reverse.ReverseImporters[0].FromPath != "consumer/use.go" {
		t.Fatalf("unexpected reverse imports: %#v", reverse.ReverseImporters)
	}

	idOffset := offsetOf(t, files["store/a.go"], "id)")
	info, err := ed.SymbolInfo(ctx, PositionSelector{Path: "store/a.go", Offset: &idOffset}, NavigationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Symbols) != 1 || info.Symbols[0].Name != "Get" {
		t.Fatalf("expected enclosing Get symbol info, got %#v", info.Symbols)
	}
}

func offsetOf(t *testing.T, src, needle string) int {
	t.Helper()
	offset := strings.Index(src, needle)
	if offset < 0 {
		t.Fatalf("needle %q not found", needle)
	}
	return offset
}

func TestRootPackageDoesNotImportGoASTPackages(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		src, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{`"go/ast"`, `"go/parser"`, `"go/token"`, `"go/format"`} {
			if strings.Contains(string(src), forbidden) {
				t.Fatalf("%s imports %s; Go parser packages must stay in internal/lang/goast", entry.Name(), forbidden)
			}
		}
	}
}

func TestCustomBackendIntegration(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditorWithOptions(t, map[string]string{
		"app.fake": "component Demo\n",
	}, WithBackend(fakeBackend{}), WithLanguage(LanguageID("fake")))

	outline, err := ed.Outline(ctx, Scope{Language: LanguageID("fake")})
	if err != nil {
		t.Fatal(err)
	}
	if len(outline.Symbols) != 1 || outline.Symbols[0].Name != "Demo" {
		t.Fatalf("unexpected fake outline: %#v", outline.Symbols)
	}

	matches, err := ed.FindSymbols(ctx, SymbolSelector{Language: LanguageID("fake"), Name: "Demo"})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one fake symbol, got %d", len(matches))
	}
}

func TestEditorWithSource(t *testing.T) {
	ctx := context.Background()
	source := mapSource{
		files: map[string][]byte{
			"main.go": []byte("package main\n\nfunc main() {}\n"),
		},
	}
	ed, err := New(".", WithSource(source), WithLanguage(Go))
	if err != nil {
		t.Fatal(err)
	}
	symbols, err := ed.FindSymbols(ctx, SymbolSelector{Name: "main", Kind: SymbolFunction})
	if err != nil {
		t.Fatal(err)
	}
	if len(symbols) != 1 {
		t.Fatalf("expected source-backed main symbol, got %#v", symbols)
	}
}

type mapSource struct {
	files map[string][]byte
}

func (s mapSource) ListFiles(ctx context.Context, scope Scope) ([]string, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	var files []string
	root := strings.Trim(scope.Path, "/")
	for file := range s.files {
		if root == "" || root == "." || file == root || strings.HasPrefix(file, root+"/") {
			files = append(files, file)
		}
	}
	return files, nil
}

func (s mapSource) ReadFile(path string) ([]byte, error) {
	data, ok := s.files[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return append([]byte(nil), data...), nil
}

func newTestEditorWithOptions(t *testing.T, files map[string]string, opts ...Option) *Editor {
	t.Helper()
	fsys := fstest.MapFS{}
	for p, src := range files {
		fsys[p] = &fstest.MapFile{Data: []byte(src)}
	}
	all := []Option{WithFS(fsys)}
	all = append(all, opts...)
	ed, err := New(".", all...)
	if err != nil {
		t.Fatal(err)
	}
	return ed
}

type fakeBackend struct{}

func (fakeBackend) Spec() BackendSpec {
	return BackendSpec{
		Language:       LanguageID("fake"),
		Name:           "fake",
		FileExtensions: []string{".fake"},
		Capabilities:   []string{"index"},
		ResolutionMode: "fake",
	}
}

func (fakeBackend) Index(ctx context.Context, snapshot core.Snapshot, scope Scope) (*core.Index, error) {
	idx := core.NewIndex()
	files, err := snapshot.ListFiles(ctx, scope)
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		if filepath.Ext(file) != ".fake" {
			continue
		}
		src, err := snapshot.ReadFile(file)
		if err != nil {
			return nil, err
		}
		name := strings.TrimSpace(strings.TrimPrefix(string(src), "component"))
		sym := Symbol{
			ID:            SymbolID(file + ":component:" + name),
			Language:      LanguageID("fake"),
			Kind:          SymbolClass,
			Name:          name,
			QualifiedName: name,
			Location:      Location{URI: file, Range: Range{Start: Position{Offset: 0}, End: Position{Offset: len(src)}}},
			Backend:       BackendInfo{Language: LanguageID("fake"), Name: "fake", ResolutionMode: "fake", Complete: true},
		}
		idx.Documents = append(idx.Documents, Document{URI: file, Language: LanguageID("fake")})
		idx.Symbols = append(idx.Symbols, sym)
		idx.ByID[sym.ID] = sym
		idx.ByName[sym.Name] = append(idx.ByName[sym.Name], sym)
	}
	return idx, nil
}

func (fakeBackend) CompileEdit(context.Context, core.Snapshot, Operation) ([]FileEdit, error) {
	return nil, errors.New("fake backend does not support edits")
}

func (fakeBackend) Format(_ context.Context, _ string, src []byte) ([]byte, error) {
	return src, nil
}

func (fakeBackend) Suggest(context.Context, core.Snapshot, Scope) ([]Proposal, error) {
	return nil, nil
}
