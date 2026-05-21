package codegate

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/codewandler/codegate/internal/core"
	"github.com/codewandler/codegate/internal/lang/goast"
)

func newTestEditor(t *testing.T, files map[string]string) *Editor {
	t.Helper()
	fsys := fstest.MapFS{}
	for p, src := range files {
		fsys[p] = &fstest.MapFile{Data: []byte(src)}
	}
	ed, err := NewEditor(".", WithFS(fsys), WithLanguage(Go))
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

func TestApplyTextEditsOrdersSameOffsetInInputOrder(t *testing.T) {
	got, err := applyTextEdits([]byte("xy"), []TextEdit{
		{
			Range:       Range{Start: Position{Offset: 2}, End: Position{Offset: 2}},
			Replacement: "C",
		},
		{
			Range:       Range{Start: Position{Offset: 1}, End: Position{Offset: 1}},
			Replacement: "A",
		},
		{
			Range:       Range{Start: Position{Offset: 1}, End: Position{Offset: 1}},
			Replacement: "B",
		},
		{
			Range:       Range{Start: Position{Offset: 0}, End: Position{Offset: 0}},
			Replacement: "^",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "^xAByC" {
		t.Fatalf("same-offset inserts should keep input order, got %q", got)
	}
}

func TestApplyTextEditsRejectsOverlappingEdits(t *testing.T) {
	_, err := applyTextEdits([]byte("abcd"), []TextEdit{
		{
			Range:       Range{Start: Position{Offset: 0}, End: Position{Offset: 2}},
			Replacement: "X",
		},
		{
			Range:       Range{Start: Position{Offset: 1}, End: Position{Offset: 3}},
			Replacement: "Y",
		},
	})
	if err == nil {
		t.Fatal("expected overlapping edits to be rejected")
	}
	if !strings.Contains(err.Error(), "overlapping text edits") {
		t.Fatalf("expected overlapping text edit error, got %v", err)
	}
}

func TestChangeSetRejectsInvalidReplaceFunctionSource(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"user.go": `package user

func CreateUser(name string) string {
	return name
}
`,
	})
	changes := ed.NewChangeSet()

	err := changes.Apply(ctx, ReplaceFunction{
		Target: SymbolSelector{Name: "CreateUser", Kind: SymbolFunction},
		Source: `func CreateUser(name string) string {
	return
`,
	})
	if err == nil {
		t.Fatal("expected invalid replacement source to fail formatting")
	}
	if !strings.Contains(err.Error(), "format user.go") {
		t.Fatalf("expected format error with path, got %v", err)
	}
	files, filesErr := changes.Files(ctx)
	if filesErr != nil {
		t.Fatal(filesErr)
	}
	if len(files) != 0 {
		t.Fatalf("invalid edit should not mark files changed: %#v", files)
	}
}

func TestGoBackendFormatReturnsInvalidGoErrors(t *testing.T) {
	ctx := context.Background()
	formatted, err := goast.New().Format(ctx, "bad.go", []byte("package bad\nfunc Broken("))
	if err == nil {
		t.Fatal("expected Go formatter to reject invalid source")
	}
	if formatted != nil {
		t.Fatalf("expected no formatted source on error, got %q", formatted)
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

func TestExecutableRefactorProposals(t *testing.T) {
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
	executable := ExecutableProposals(proposals)
	if len(executable) == 0 {
		t.Fatalf("expected executable proposals: %#v", proposals)
	}
	for _, proposal := range executable {
		if !HasOperations(proposal) {
			t.Fatalf("executable proposal without operations: %#v", proposal)
		}
	}
	var deleteUnused Proposal
	for _, proposal := range executable {
		if proposal.Kind != RefactorDeleteSymbol {
			continue
		}
		for _, target := range proposal.Targets {
			if target.Name == "unusedHelper" {
				deleteUnused = proposal
				break
			}
		}
	}
	if len(deleteUnused.Operations) != 1 {
		t.Fatalf("expected delete proposal for unusedHelper with one operation: %#v", executable)
	}
	deleteOp, ok := deleteUnused.Operations[0].(DeleteSymbol)
	if !ok || deleteOp.ExpectedHash == "" {
		t.Fatalf("expected hash-guarded DeleteSymbol operation, got %#v", deleteUnused.Operations[0])
	}
	changes := ed.NewChangeSet()
	if err := changes.Apply(ctx, deleteUnused.Operations...); err != nil {
		t.Fatal(err)
	}
	after := string(mustFiles(t, changes, ctx)[0].After)
	if strings.Contains(after, "unusedHelper") {
		t.Fatalf("proposal operation did not delete unusedHelper:\n%s", after)
	}
}

func TestAdvisoryRefactorProposalsHaveNoOperations(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"demo.go": `package demo

func Render(name string, compact bool, a int, b int, c int) {}
`,
	})
	proposals, err := ed.SuggestRefactorings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, proposal := range proposals {
		switch proposal.Kind {
		case RefactorIntroduceConfig, RefactorReplaceFlagArgument:
			if HasOperations(proposal) {
				t.Fatalf("advisory proposal should not include operations: %#v", proposal)
			}
			found := false
			for _, evidence := range proposal.Evidence {
				if evidence.Kind == "advisory_no_operation" {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("advisory proposal missing advisory evidence: %#v", proposal)
			}
		}
	}
}

func TestSuggestRefactoringsSkipsGoEntrypoints(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"main.go": `package main

func init() {}

func main() {}
`,
		"cmd/app/main.go": `package main

func init() {}

func main() {}

func unusedHelper() {}
`,
	})
	proposals, err := ed.SuggestRefactorings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	deleted := map[string]bool{}
	for _, proposal := range proposals {
		if proposal.Kind != RefactorDeleteSymbol {
			continue
		}
		for _, target := range proposal.Targets {
			deleted[target.Name] = true
		}
	}
	if deleted["main"] || deleted["init"] {
		t.Fatalf("entrypoints should not be delete suggestions: %#v", proposals)
	}
	if !deleted["unusedHelper"] {
		t.Fatalf("expected unused helper delete suggestion: %#v", proposals)
	}
}

func TestRenameSymbolRenamesFunctionAndCalls(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"a.go": `package demo

func Target() {}

func Caller() {
	Target()
}
`,
		"b.go": `package demo

func Other() {
	Target()
}
`,
	})
	fragment, err := ed.ReadSymbol(ctx, SymbolSelector{Name: "Target", Kind: SymbolFunction})
	if err != nil {
		t.Fatal(err)
	}
	changes := ed.NewChangeSet()
	if err := changes.Apply(ctx, RenameSymbol{
		Target:       SymbolSelector{ID: fragment.Symbol.ID},
		NewName:      "Renamed",
		ExpectedHash: fragment.Hash,
	}); err != nil {
		t.Fatal(err)
	}
	files, err := changes.Files(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := changedFilesByPath(files)
	if !strings.Contains(string(got["a.go"].After), "func Renamed()") || !strings.Contains(string(got["a.go"].After), "Renamed()") {
		t.Fatalf("a.go was not renamed correctly:\n%s", got["a.go"].After)
	}
	if !strings.Contains(string(got["b.go"].After), "Renamed()") {
		t.Fatalf("b.go was not renamed correctly:\n%s", got["b.go"].After)
	}
}

func TestRenameSymbolRenamesMethodAndDirectSelectorCalls(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"store.go": `package demo

type Store struct{}

func (s *Store) Load() {}

func Caller(s *Store) {
	s.Load()
}
`,
	})
	changes := ed.NewChangeSet()
	if err := changes.Apply(ctx, RenameSymbol{
		Target:  SymbolSelector{Name: "Load", Kind: SymbolMethod, Container: "Store"},
		NewName: "Fetch",
	}); err != nil {
		t.Fatal(err)
	}
	files, err := changes.Files(ctx)
	if err != nil {
		t.Fatal(err)
	}
	after := string(files[0].After)
	if !strings.Contains(after, "func (s *Store) Fetch()") || !strings.Contains(after, "s.Fetch()") {
		t.Fatalf("method was not renamed correctly:\n%s", after)
	}
}

func TestRenameSymbolRenamesTypeUsages(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"types.go": `package demo

type Thing struct{}

func NewThing() Thing {
	return Thing{}
}
`,
	})
	changes := ed.NewChangeSet()
	if err := changes.Apply(ctx, RenameSymbol{
		Target:  SymbolSelector{Name: "Thing", Kind: SymbolStruct},
		NewName: "Widget",
	}); err != nil {
		t.Fatal(err)
	}
	files, err := changes.Files(ctx)
	if err != nil {
		t.Fatal(err)
	}
	after := string(files[0].After)
	if !strings.Contains(after, "type Widget struct{}") || !strings.Contains(after, "func NewThing() Widget") || !strings.Contains(after, "return Widget{}") {
		t.Fatalf("type usages were not renamed correctly:\n%s", after)
	}
}

func TestRenameSymbolRejectsInvalidAndUnsupportedTargets(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"user.go": `package demo

type User struct {
	Email string
}

func Target() {}
`,
	})
	changes := ed.NewChangeSet()
	if err := changes.Apply(ctx, RenameSymbol{Target: SymbolSelector{Name: "Target", Kind: SymbolFunction}, NewName: "1bad"}); err == nil {
		t.Fatal("expected invalid identifier to be rejected")
	}
	if err := changes.Apply(ctx, RenameSymbol{Target: SymbolSelector{Name: "Email", Kind: SymbolField}, NewName: "Address"}); err == nil {
		t.Fatal("expected field rename to be rejected")
	}
	files, err := changes.Files(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("rejected rename should not change files: %#v", files)
	}
}

func TestRenameSymbolRejectsAmbiguousSelector(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"a/a.go": `package a

func Target() {}
`,
		"b/b.go": `package b

func Target() {}
`,
	})
	changes := ed.NewChangeSet()
	err := changes.Apply(ctx, RenameSymbol{
		Target:  SymbolSelector{Name: "Target", Kind: SymbolFunction},
		NewName: "Renamed",
	})
	if err == nil {
		t.Fatal("expected ambiguous rename to fail")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected ambiguity error, got %v", err)
	}
}

func TestGoOccurrenceKindsAndReferenceEdges(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"demo.go": `package demo

import "fmt"

// Counter tracks calls.
var Counter = 0

type User struct {
	Email string
}

func Target() {}

func Use(user User) string {
	Counter = Counter + 1
	Counter++
	user.Email = fmt.Sprint(Counter)
	Target()
	return user.Email
}
`,
	})
	idx, err := ed.buildIndex(ctx, Scope{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	counter := onlySymbol(t, idx, "Counter", SymbolVar)
	email := onlySymbol(t, idx, "Email", SymbolField)
	target := onlySymbol(t, idx, "Target", SymbolFunction)

	kinds := occurrencesByKind(idx.Occurrences)
	if kinds[OccurrenceImport] == 0 {
		t.Fatalf("expected import occurrence: %#v", idx.Occurrences)
	}
	if kinds[OccurrenceDoc] == 0 {
		t.Fatalf("expected doc occurrence: %#v", idx.Occurrences)
	}
	if countOccurrenceKind(idx.Occurrences, counter.ID, OccurrenceWrite) < 2 {
		t.Fatalf("expected Counter writes: %#v", idx.Occurrences)
	}
	if countOccurrenceKind(idx.Occurrences, counter.ID, OccurrenceRead) < 2 {
		t.Fatalf("expected Counter reads: %#v", idx.Occurrences)
	}
	if countOccurrenceKind(idx.Occurrences, email.ID, OccurrenceWrite) != 1 || countOccurrenceKind(idx.Occurrences, email.ID, OccurrenceRead) != 1 {
		t.Fatalf("expected Email read/write occurrences: %#v", idx.Occurrences)
	}
	if countOccurrenceKind(idx.Occurrences, target.ID, OccurrenceCall) != 1 {
		t.Fatalf("expected Target call occurrence: %#v", idx.Occurrences)
	}
	foundReferenceEdge := false
	for _, edge := range idx.Edges {
		if edge.Kind == EdgeReferences && edge.To == string(counter.ID) {
			foundReferenceEdge = true
			break
		}
	}
	if !foundReferenceEdge {
		t.Fatalf("expected reference edge for Counter: %#v", idx.Edges)
	}
}

func TestExpectedHashGuardsSemanticEdits(t *testing.T) {
	ctx := context.Background()
	for name, apply := range map[string]func(*ChangeSet) error{
		"replace": func(changes *ChangeSet) error {
			return changes.Apply(ctx, ReplaceFunction{
				Target:       SymbolSelector{Name: "Target", Kind: SymbolFunction},
				Source:       "func Target() string { return \"new\" }",
				ExpectedHash: "stale",
			})
		},
		"delete": func(changes *ChangeSet) error {
			return changes.Apply(ctx, DeleteSymbol{
				Target:       SymbolSelector{Name: "Target", Kind: SymbolFunction},
				ExpectedHash: "stale",
			})
		},
		"comment": func(changes *ChangeSet) error {
			return changes.Apply(ctx, ReplaceComment{
				Target:       SymbolSelector{Name: "Target", Kind: SymbolFunction},
				Text:         "Target is updated.",
				ExpectedHash: "stale",
			})
		},
		"rename": func(changes *ChangeSet) error {
			return changes.Apply(ctx, RenameSymbol{
				Target:       SymbolSelector{Name: "Target", Kind: SymbolFunction},
				NewName:      "Renamed",
				ExpectedHash: "stale",
			})
		},
	} {
		t.Run(name, func(t *testing.T) {
			ed := newTestEditor(t, map[string]string{
				"target.go": `package demo

func Target() string {
	return "old"
}
`,
			})
			changes := ed.NewChangeSet()
			err := apply(changes)
			if err == nil {
				t.Fatal("expected stale hash error")
			}
			if !strings.Contains(err.Error(), "stale source") {
				t.Fatalf("expected stale source error, got %v", err)
			}
			files, filesErr := changes.Files(ctx)
			if filesErr != nil {
				t.Fatal(filesErr)
			}
			if len(files) != 0 {
				t.Fatalf("stale edit should not change files: %#v", files)
			}
		})
	}
}

func TestExpectedHashAllowsMatchingReplaceFunction(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"target.go": `package demo

func Target() string {
	return "old"
}
`,
	})
	fragment, err := ed.ReadSymbol(ctx, SymbolSelector{Name: "Target", Kind: SymbolFunction})
	if err != nil {
		t.Fatal(err)
	}
	changes := ed.NewChangeSet()
	if err := changes.Apply(ctx, ReplaceFunction{
		Target:       SymbolSelector{ID: fragment.Symbol.ID},
		Source:       "func Target() string { return \"new\" }",
		ExpectedHash: fragment.Hash,
	}); err != nil {
		t.Fatal(err)
	}
	files, err := changes.Files(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || !strings.Contains(string(files[0].After), `return "new"`) {
		t.Fatalf("matching hash should allow replacement: %#v", files)
	}
}

func TestDeleteSymbolDeletesOnlySelectedGroupedSpec(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"values.go": `package demo

const (
	Keep = 1
	Drop = 2
	AlsoKeep = 3
)

type (
	KeepType struct{}
	DropType struct{}
)
`,
	})
	changes := ed.NewChangeSet()
	if err := changes.Apply(ctx,
		DeleteSymbol{Target: SymbolSelector{Name: "Drop", Kind: SymbolConst}},
		DeleteSymbol{Target: SymbolSelector{Name: "DropType", Kind: SymbolStruct}},
	); err != nil {
		t.Fatal(err)
	}
	files, err := changes.Files(ctx)
	if err != nil {
		t.Fatal(err)
	}
	after := string(files[0].After)
	for _, want := range []string{"Keep", "AlsoKeep = 3", "KeepType struct{}"} {
		if !strings.Contains(after, want) {
			t.Fatalf("expected %q to remain:\n%s", want, after)
		}
	}
	for _, gone := range []string{"Drop = 2", "DropType struct{}"} {
		if strings.Contains(after, gone) {
			t.Fatalf("expected %q to be deleted:\n%s", gone, after)
		}
	}
}

func TestReplaceAndAppendSymbolWrappers(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"a.go": `package demo

type User struct{}
`,
		"b.go": `package demo
`,
	})
	changes := ed.NewChangeSet()
	if err := changes.Apply(ctx,
		ReplaceSymbol{
			Target: SymbolSelector{Name: "User", Kind: SymbolStruct},
			Source: "type Account struct{}",
		},
		AppendSymbol{
			UnitID: "demo",
			Source: "const Added = 1",
		},
		AppendSymbol{
			Path:   "b.go",
			Source: "var Other = 2",
		},
	); err != nil {
		t.Fatal(err)
	}
	got := changedFilesByPath(mustFiles(t, changes, ctx))
	if !strings.Contains(string(got["a.go"].After), "type Account struct{}") || !strings.Contains(string(got["a.go"].After), "const Added = 1") {
		t.Fatalf("a.go wrapper edits failed:\n%s", got["a.go"].After)
	}
	if !strings.Contains(string(got["b.go"].After), "var Other = 2") {
		t.Fatalf("b.go append failed:\n%s", got["b.go"].After)
	}
}

func TestMethodAndFunctionWrappersEnforceKinds(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"demo.go": `package demo

type Store struct{}

func Target() {}

func (Store) Load() {}
`,
	})
	changes := ed.NewChangeSet()
	if err := changes.Apply(ctx, ReplaceMethod{
		Target: SymbolSelector{Name: "Load", Container: "Store"},
		Source: "func (Store) Fetch() {}",
	}); err != nil {
		t.Fatal(err)
	}
	if err := changes.Apply(ctx, DeleteFunction{Target: SymbolSelector{Name: "Target"}}); err != nil {
		t.Fatal(err)
	}
	if err := changes.Apply(ctx, DeleteMethod{Target: SymbolSelector{Name: "Fetch", Container: "Store"}}); err != nil {
		t.Fatal(err)
	}
	if err := changes.Apply(ctx, ReplaceMethod{Target: SymbolSelector{Name: "Target", Kind: SymbolFunction}, Source: "func Target() {}"}); err == nil {
		t.Fatal("expected ReplaceMethod to reject function kind")
	}
}

func TestGoImportEdits(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"empty.go": `package demo

func A() {}
`,
		"single.go": `package demo

import "fmt"

func B() {}
`,
		"group.go": `package demo

import (
	"fmt"
	alias "strings"
)

func C() {}
`,
	})
	changes := ed.NewChangeSet()
	if err := changes.Apply(ctx,
		EnsureGoImport{Path: "empty.go", ImportPath: "strings"},
		EnsureGoImport{Path: "single.go", ImportPath: "strings"},
		EnsureGoImport{Path: "single.go", ImportPath: "strings"},
		RemoveGoImport{Path: "group.go", ImportPath: "fmt"},
		RenameGoImport{Path: "group.go", ImportPath: "strings", Alias: "textstrings"},
	); err != nil {
		t.Fatal(err)
	}
	got := changedFilesByPath(mustFiles(t, changes, ctx))
	if !strings.Contains(string(got["empty.go"].After), `import "strings"`) {
		t.Fatalf("empty import insert failed:\n%s", got["empty.go"].After)
	}
	if strings.Count(string(got["single.go"].After), `"strings"`) != 1 || !strings.Contains(string(got["single.go"].After), "import (") {
		t.Fatalf("single import ensure failed:\n%s", got["single.go"].After)
	}
	groupAfter := string(got["group.go"].After)
	if strings.Contains(groupAfter, `"fmt"`) || !strings.Contains(groupAfter, `textstrings "strings"`) {
		t.Fatalf("group import remove/rename failed:\n%s", groupAfter)
	}
}

func TestRenameGoModulePathUpdatesGoModAndImports(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"go.mod": "module github.com/acme/old\n\ngo 1.24\n",
		"app.go": `package demo

import (
	alias "github.com/acme/old/internal/pkg" // keep comment
	"github.com/acme/old/lib"
	"github.com/other/module"
)

var _ = alias.Name
var _ = lib.Name
`,
		"app_test.go": `package demo

import "github.com/acme/old/testkit"

var _ = testkit.Name
`,
		"vendor/github.com/acme/old/v.go": `package vendor

import "github.com/acme/old/shouldnotchange"
`,
	})
	changes := ed.NewChangeSet()
	if err := changes.Apply(ctx, RenameGoModulePath{
		OldPath: "github.com/acme/old",
		NewPath: "github.com/acme/new",
	}); err != nil {
		t.Fatal(err)
	}
	got := changedFilesByPath(mustFiles(t, changes, ctx))
	if !strings.Contains(string(got["go.mod"].After), "module github.com/acme/new") {
		t.Fatalf("go.mod was not renamed:\n%s", got["go.mod"].After)
	}
	appAfter := string(got["app.go"].After)
	for _, want := range []string{
		`alias "github.com/acme/new/internal/pkg"`,
		`"github.com/acme/new/lib"`,
		`"github.com/other/module"`,
		"// keep comment",
	} {
		if !strings.Contains(appAfter, want) {
			t.Fatalf("app.go missing %q:\n%s", want, appAfter)
		}
	}
	testAfter := string(got["app_test.go"].After)
	if !strings.Contains(testAfter, `"github.com/acme/new/testkit"`) {
		t.Fatalf("test import was not renamed:\n%s", testAfter)
	}
	if _, ok := got["vendor/github.com/acme/old/v.go"]; ok {
		t.Fatalf("vendor file should not be edited: %#v", got)
	}
}

func TestRenameGoModulePathRejectsUnsafeInputs(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"go.mod": "module github.com/acme/current\n\ngo 1.24\n",
		"a.go":   "package demo\n",
	})
	for name, op := range map[string]RenameGoModulePath{
		"mismatched old path": {OldPath: "github.com/acme/other", NewPath: "github.com/acme/new"},
		"invalid new path":    {OldPath: "github.com/acme/current", NewPath: "../bad"},
		"same path":           {OldPath: "github.com/acme/current", NewPath: "github.com/acme/current"},
	} {
		t.Run(name, func(t *testing.T) {
			changes := ed.NewChangeSet()
			if err := changes.Apply(ctx, op); err == nil {
				t.Fatal("expected module rename to fail")
			}
		})
	}
}

func TestRenameGoModulePathRejectsImportCollisions(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"go.mod": "module github.com/acme/old\n\ngo 1.24\n",
		"a.go": `package demo

import (
	"github.com/acme/new/lib"
	"github.com/acme/old/lib"
)
`,
	})
	changes := ed.NewChangeSet()
	if err := changes.Apply(ctx, RenameGoModulePath{OldPath: "github.com/acme/old", NewPath: "github.com/acme/new"}); err == nil {
		t.Fatal("expected duplicate import collision to fail")
	}
}

func TestRenameGoModulePathAllowsLocalModuleNames(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"go.mod": "module localmod\n\ngo 1.24\n",
		"a.go": `package demo

import "localmod/pkg"

var _ = pkg.Name
`,
		"pkg/pkg.go": `package pkg

const Name = "pkg"
`,
	})
	changes := ed.NewChangeSet()
	if err := changes.Apply(ctx, RenameGoModulePath{OldPath: "localmod", NewPath: "newlocalmod"}); err != nil {
		t.Fatal(err)
	}
	got := changedFilesByPath(mustFiles(t, changes, ctx))
	if !strings.Contains(string(got["go.mod"].After), "module newlocalmod") || !strings.Contains(string(got["a.go"].After), `"newlocalmod/pkg"`) {
		t.Fatalf("local module rename failed: %#v", got)
	}
}

func TestGoImportEditsByUnit(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"a/a.go": `package a

func A() {}
`,
		"a/b.go": `package a

func B() {}
`,
	})
	changes := ed.NewChangeSet()
	if err := changes.Apply(ctx, EnsureGoImport{UnitID: "a#a", ImportPath: "fmt"}); err != nil {
		t.Fatal(err)
	}
	files := mustFiles(t, changes, ctx)
	if len(files) != 1 || files[0].Path != "a/a.go" || !strings.Contains(string(files[0].After), `import "fmt"`) {
		t.Fatalf("unit import edit should target first sorted unit file: %#v", files)
	}
}

func TestMoveSymbolMovesFunctionAndType(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"from.go": `package demo

const (
	Keep = 1
	MoveMe = 2
)

func Run() string {
	return "ok"
}
`,
		"to.go": `package demo

func Existing() {}
`,
	})
	fragment, err := ed.ReadSymbol(ctx, SymbolSelector{Name: "Run", Kind: SymbolFunction})
	if err != nil {
		t.Fatal(err)
	}
	changes := ed.NewChangeSet()
	if err := changes.Apply(ctx,
		MoveSymbol{Target: SymbolSelector{ID: fragment.Symbol.ID}, ToPath: "to.go", ExpectedHash: fragment.Hash},
		MoveSymbol{Target: SymbolSelector{Name: "MoveMe", Kind: SymbolConst}, ToPath: "to.go"},
	); err != nil {
		t.Fatal(err)
	}
	got := changedFilesByPath(mustFiles(t, changes, ctx))
	fromAfter := string(got["from.go"].After)
	toAfter := string(got["to.go"].After)
	if strings.Contains(fromAfter, "func Run") || strings.Contains(fromAfter, "MoveMe") || !strings.Contains(fromAfter, "Keep = 1") {
		t.Fatalf("source move edits failed:\n%s", fromAfter)
	}
	if !strings.Contains(toAfter, "func Run() string") || !strings.Contains(toAfter, "MoveMe = 2") {
		t.Fatalf("target move edits failed:\n%s", toAfter)
	}
}

func TestMoveSymbolRejectsStaleAndUnsupported(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"from.go": `package demo

type User struct {
	Email string
}

func Run() {}
`,
		"to.go": `package demo
`,
	})
	changes := ed.NewChangeSet()
	if err := changes.Apply(ctx, MoveSymbol{Target: SymbolSelector{Name: "Run", Kind: SymbolFunction}, ToPath: "to.go", ExpectedHash: "stale"}); err == nil {
		t.Fatal("expected stale move to fail")
	}
	if err := changes.Apply(ctx, MoveSymbol{Target: SymbolSelector{Name: "Email", Kind: SymbolField}, ToPath: "to.go"}); err == nil {
		t.Fatal("expected field move to fail")
	}
	files, err := changes.Files(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("rejected moves should not change files: %#v", files)
	}
}

func TestMoveSymbolReconcilesImports(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"from.go": `package demo

import (
	"fmt"
	"sort"
	"strings"
)

func Keep() string {
	return strings.TrimSpace(" keep ")
}

func MoveMe(values []string) string {
	sort.Strings(values)
	return fmt.Sprint(values)
}
`,
		"to.go": `package demo

func Existing() {}
`,
	})
	changes := ed.NewChangeSet()
	if err := changes.Apply(ctx, MoveSymbol{
		Target:           SymbolSelector{Name: "MoveMe", Kind: SymbolFunction},
		ToPath:           "to.go",
		ReconcileImports: true,
	}); err != nil {
		t.Fatal(err)
	}
	got := changedFilesByPath(mustFiles(t, changes, ctx))
	fromAfter := string(got["from.go"].After)
	toAfter := string(got["to.go"].After)
	if strings.Contains(fromAfter, `"fmt"`) || strings.Contains(fromAfter, `"sort"`) || !strings.Contains(fromAfter, `"strings"`) {
		t.Fatalf("source imports were not reconciled:\n%s", fromAfter)
	}
	if !strings.Contains(toAfter, `"fmt"`) || !strings.Contains(toAfter, `"sort"`) || !strings.Contains(toAfter, "func MoveMe") {
		t.Fatalf("target import/function move failed:\n%s", toAfter)
	}
}

func TestGoParameterOperationsUpdateDeclarationsAndCalls(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"a.go": `package demo

func Target(name string) string {
	return name
}

func Caller() {
	_ = Target("a")
}
`,
		"b.go": `package demo

func Other() {
	_ = Target("b")
}
`,
	})
	changes := ed.NewChangeSet()
	if err := changes.Apply(ctx, AddGoParameter{
		Target:       SymbolSelector{Name: "Target", Kind: SymbolFunction},
		Name:         "active",
		Type:         "bool",
		DefaultValue: "true",
		Position:     1,
	}); err != nil {
		t.Fatal(err)
	}
	got := changedFilesByPath(mustFiles(t, changes, ctx))
	if !strings.Contains(string(got["a.go"].After), "func Target(name string, active bool) string") || !strings.Contains(string(got["a.go"].After), `Target("a", true)`) {
		t.Fatalf("add parameter did not update declaration and same-file call:\n%s", got["a.go"].After)
	}
	if !strings.Contains(string(got["b.go"].After), `Target("b", true)`) {
		t.Fatalf("add parameter did not update cross-file call:\n%s", got["b.go"].After)
	}

	if err := changes.Apply(ctx, RemoveGoParameter{
		Target: SymbolSelector{Name: "Target", Kind: SymbolFunction},
		Name:   "active",
	}); err != nil {
		t.Fatal(err)
	}
	got = changedFilesByPath(mustFiles(t, changes, ctx))
	if strings.Contains(string(got["a.go"].After), "active bool") || strings.Contains(string(got["a.go"].After), `Target("a", true)`) {
		t.Fatalf("remove parameter did not update declaration and call:\n%s", got["a.go"].After)
	}
	if strings.Contains(string(got["b.go"].After), `Target("b", true)`) {
		t.Fatalf("remove parameter did not update cross-file call:\n%s", got["b.go"].After)
	}
}

func TestRenameGoParameterUpdatesBodyReferences(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"demo.go": `package demo

func Target(name string) string {
	value := name
	return value + name
}
`,
	})
	changes := ed.NewChangeSet()
	if err := changes.Apply(ctx, RenameGoParameter{
		Target:  SymbolSelector{Name: "Target", Kind: SymbolFunction},
		OldName: "name",
		NewName: "label",
	}); err != nil {
		t.Fatal(err)
	}
	after := string(mustFiles(t, changes, ctx)[0].After)
	if !strings.Contains(after, "func Target(label string) string") || strings.Contains(after, " name") || !strings.Contains(after, "value := label") || !strings.Contains(after, "value + label") {
		t.Fatalf("parameter rename failed:\n%s", after)
	}
}

func TestGoParameterRenameRejectsShadowedBodyNames(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"demo.go": `package demo

func Target(name string) string {
	if name != "" {
		name := "shadow"
		return name
	}
	return name
}
`,
	})
	changes := ed.NewChangeSet()
	err := changes.Apply(ctx, RenameGoParameter{
		Target:  SymbolSelector{Name: "Target", Kind: SymbolFunction},
		OldName: "name",
		NewName: "label",
	})
	if err == nil {
		t.Fatal("expected shadowed parameter rename to fail")
	}
	if !strings.Contains(err.Error(), "shadowed") {
		t.Fatalf("expected shadowing error, got %v", err)
	}
}

func TestGoSignatureChangeRejectsFunctionValueUse(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"demo.go": `package demo

func Target(name string) string {
	return name
}

func Caller() {
	fn := Target
	_ = fn("a")
}
`,
	})
	changes := ed.NewChangeSet()
	err := changes.Apply(ctx, AddGoParameter{
		Target:       SymbolSelector{Name: "Target", Kind: SymbolFunction},
		Name:         "active",
		Type:         "bool",
		DefaultValue: "true",
		Position:     1,
	})
	if err == nil {
		t.Fatal("expected function-value signature change to fail")
	}
	if !strings.Contains(err.Error(), "function value") {
		t.Fatalf("expected function value error, got %v", err)
	}
}

func TestRemoveGoParameterHandlesGroupedNames(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"demo.go": `package demo

func Grouped(first, second string, count int) {}

func Caller() {
	Grouped("a", "b", 1)
}
`,
	})
	changes := ed.NewChangeSet()
	if err := changes.Apply(ctx, RemoveGoParameter{
		Target: SymbolSelector{Name: "Grouped", Kind: SymbolFunction},
		Name:   "second",
	}); err != nil {
		t.Fatal(err)
	}
	after := string(mustFiles(t, changes, ctx)[0].After)
	if !strings.Contains(after, "func Grouped(first string, count int)") || !strings.Contains(after, `Grouped("a", 1)`) || strings.Contains(after, "second") {
		t.Fatalf("grouped parameter removal failed:\n%s", after)
	}
}

func TestGoStructFieldOperations(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"user.go": `package demo

type User struct {
	Name string
	First, Last string
}
`,
	})
	changes := ed.NewChangeSet()
	if err := changes.Apply(ctx, AddGoStructField{
		Struct:   SymbolSelector{Name: "User", Kind: SymbolStruct},
		Name:     "Email",
		Type:     "string",
		Tag:      `json:"email"`,
		Comment:  "Email stores the primary address.",
		Position: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := changes.Apply(ctx, RemoveGoStructField{
		Struct: SymbolSelector{Name: "User", Kind: SymbolStruct},
		Field:  "Last",
	}); err != nil {
		t.Fatal(err)
	}
	after := string(mustFiles(t, changes, ctx)[0].After)
	if !strings.Contains(after, "Name string") || strings.Contains(after, "Last") || !strings.Contains(after, "First string") || !strings.Contains(after, "Email string `json:\"email\"`") || !strings.Contains(after, "// Email stores the primary address.") {
		t.Fatalf("struct field operations failed:\n%s", after)
	}
}

func TestRemoveGoStructFieldRejectsReferencedField(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"user.go": `package demo

type User struct {
	Email string
}

func Use(user User) string {
	return user.Email
}
`,
	})
	changes := ed.NewChangeSet()
	err := changes.Apply(ctx, RemoveGoStructField{
		Struct: SymbolSelector{Name: "User", Kind: SymbolStruct},
		Field:  "Email",
	})
	if err == nil {
		t.Fatal("expected referenced field removal to fail")
	}
	if !strings.Contains(err.Error(), "indexed references") {
		t.Fatalf("expected reference error, got %v", err)
	}
}

func TestRenameGoStructFieldUpdatesKeyedLiteralsAndOptionalSelectors(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"user.go": `package demo

type User struct {
	Email string
	First, Last string
}

func NewUser() User {
	return User{Email: "a", Last: "b"}
}

func Read(user User) string {
	return user.Email
}
`,
	})
	changes := ed.NewChangeSet()
	if err := changes.Apply(ctx, RenameGoStructField{
		Struct:          SymbolSelector{Name: "User", Kind: SymbolStruct},
		OldName:         "Email",
		NewName:         "Address",
		UpdateSelectors: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := changes.Apply(ctx, RenameGoStructField{
		Struct:  SymbolSelector{Name: "User", Kind: SymbolStruct},
		OldName: "Last",
		NewName: "Surname",
	}); err != nil {
		t.Fatal(err)
	}
	after := string(mustFiles(t, changes, ctx)[0].After)
	for _, want := range []string{
		"Address",
		"First, Surname string",
		"User{Address: \"a\", Surname: \"b\"}",
		"return user.Address",
	} {
		if !strings.Contains(after, want) {
			t.Fatalf("field rename output missing %q:\n%s", want, after)
		}
	}
	for _, gone := range []string{"Email string", "Last string", "Email:", "Last:", "user.Email"} {
		if strings.Contains(after, gone) {
			t.Fatalf("field rename output still contains %q:\n%s", gone, after)
		}
	}
}

func TestRenameGoStructFieldRejectsAmbiguousSelectorOwnership(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"demo.go": `package demo

type User struct {
	Email string
}

type Account struct {
	Email string
}

func Read(user User, account Account) string {
	return user.Email + account.Email
}
`,
	})
	changes := ed.NewChangeSet()
	err := changes.Apply(ctx, RenameGoStructField{
		Struct:          SymbolSelector{Name: "User", Kind: SymbolStruct},
		OldName:         "Email",
		NewName:         "Address",
		UpdateSelectors: true,
	})
	if err == nil {
		t.Fatal("expected ambiguous field selector rename to fail")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected ambiguity error, got %v", err)
	}
}

func TestChangeGoSignatureTypesAndReceiver(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"demo.go": `package demo

type Store struct{}

func Target(name string, count int) (value string, err error) {
	return name, nil
}

func (s *Store) Load(id string) string {
	return s.load(id)
}

func (s *Store) load(id string) string {
	return id
}
`,
	})
	changes := ed.NewChangeSet()
	if err := changes.Apply(ctx,
		ChangeGoParameterType{
			Target: SymbolSelector{Name: "Target", Kind: SymbolFunction},
			Name:   "count",
			Type:   "int64",
		},
		ChangeGoResultType{
			Target: SymbolSelector{Name: "Target", Kind: SymbolFunction},
			Name:   "value",
			Type:   "[]byte",
		},
		RenameGoReceiver{
			Target:  SymbolSelector{Name: "Load", Kind: SymbolMethod, Container: "Store"},
			NewName: "store",
		},
	); err != nil {
		t.Fatal(err)
	}
	after := string(mustFiles(t, changes, ctx)[0].After)
	for _, want := range []string{
		"func Target(name string, count int64) (value []byte, err error)",
		"func (store *Store) Load(id string) string",
		"return store.load(id)",
		"func (s *Store) load(id string) string",
	} {
		if !strings.Contains(after, want) {
			t.Fatalf("signature/receiver output missing %q:\n%s", want, after)
		}
	}
}

func TestChangeGoResultTypeByPosition(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"demo.go": `package demo

func Target() (string, error) {
	return "", nil
}
`,
	})
	changes := ed.NewChangeSet()
	if err := changes.Apply(ctx, ChangeGoResultType{
		Target:   SymbolSelector{Name: "Target", Kind: SymbolFunction},
		Position: 0,
		Type:     "[]byte",
	}); err != nil {
		t.Fatal(err)
	}
	after := string(mustFiles(t, changes, ctx)[0].After)
	if !strings.Contains(after, "func Target() ([]byte, error)") {
		t.Fatalf("positional result type change failed:\n%s", after)
	}
}

func TestChangeGoParameterTypeRejectsGroupedNames(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"demo.go": `package demo

func Target(first, second string) {}
`,
	})
	changes := ed.NewChangeSet()
	err := changes.Apply(ctx, ChangeGoParameterType{
		Target: SymbolSelector{Name: "Target", Kind: SymbolFunction},
		Name:   "second",
		Type:   "[]byte",
	})
	if err == nil {
		t.Fatal("expected grouped parameter type change to fail")
	}
	if !strings.Contains(err.Error(), "grouped parameter") {
		t.Fatalf("expected grouped parameter error, got %v", err)
	}
}

func TestGoInterfaceMethodOperations(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"demo.go": `package demo

type Store interface {
	Get(id string) string
	Delete(id string) error
}
`,
	})
	changes := ed.NewChangeSet()
	if err := changes.Apply(ctx, AddGoInterfaceMethod{
		Interface: SymbolSelector{Name: "Store", Kind: SymbolInterface},
		Method:    "List() []string",
		Position:  1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := changes.Apply(ctx, RemoveGoInterfaceMethod{
		Interface: SymbolSelector{Name: "Store", Kind: SymbolInterface},
		Method:    "Delete",
	}); err != nil {
		t.Fatal(err)
	}
	after := string(mustFiles(t, changes, ctx)[0].After)
	if !strings.Contains(after, "Get(id string) string") || !strings.Contains(after, "List() []string") || strings.Contains(after, "Delete(id string) error") {
		t.Fatalf("interface method operations failed:\n%s", after)
	}
}

func TestRemoveGoInterfaceMethodRejectsReferences(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"demo.go": `package demo

type Store interface {
	Get(id string) string
}

func Use(store Store) string {
	return store.Get("a")
}
`,
	})
	changes := ed.NewChangeSet()
	err := changes.Apply(ctx, RemoveGoInterfaceMethod{
		Interface: SymbolSelector{Name: "Store", Kind: SymbolInterface},
		Method:    "Get",
	})
	if err == nil {
		t.Fatal("expected referenced interface method removal to fail")
	}
	if !strings.Contains(err.Error(), "indexed references") {
		t.Fatalf("expected reference error, got %v", err)
	}
}

func TestNewGoRefactorsRejectStaleAndConflictingEdits(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"demo.go": `package demo

type User struct {
	Email string
	Name string
}

func Target(name string) string {
	return name
}
`,
	})
	changes := ed.NewChangeSet()
	if err := changes.Apply(ctx, ChangeGoParameterType{
		Target:       SymbolSelector{Name: "Target", Kind: SymbolFunction},
		Name:         "name",
		Type:         "[]byte",
		ExpectedHash: "stale",
	}); err == nil {
		t.Fatal("expected stale signature type change to fail")
	}
	if err := changes.Apply(ctx, RenameGoStructField{
		Struct:  SymbolSelector{Name: "User", Kind: SymbolStruct},
		OldName: "Email",
		NewName: "Name",
	}); err == nil {
		t.Fatal("expected conflicting field rename to fail")
	}
	if files := mustFiles(t, changes, ctx); len(files) != 0 {
		t.Fatalf("rejected refactors should not change files: %#v", files)
	}
}

func TestGoRefactorsRejectGeneratedFiles(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"generated.go": `// Code generated by test. DO NOT EDIT.
package demo

func Target() {}
`,
	})
	changes := ed.NewChangeSet()
	err := changes.Apply(ctx, DeleteFunction{Target: SymbolSelector{Name: "Target", Kind: SymbolFunction}})
	if err == nil {
		t.Fatal("expected generated-file refactor to fail")
	}
	if !strings.Contains(err.Error(), "generated Go file") {
		t.Fatalf("expected generated-file error, got %v", err)
	}
}

func TestValidationReportsParseAndTypecheckDiagnostics(t *testing.T) {
	ctx := context.Background()
	parseEditor := newTestEditor(t, map[string]string{
		"bad.go": `package demo

func Bad( {
}
`,
	})
	parseResult, err := parseEditor.Validate(ctx, ValidationOptions{Kinds: []ValidationKind{ValidationParse}})
	if err != nil {
		t.Fatal(err)
	}
	if parseResult.Passed || len(parseResult.Diagnostics) == 0 || parseResult.ResolutionMode != "ast" {
		t.Fatalf("expected parse validation failure, got %#v", parseResult)
	}

	typeEditor := newTestEditor(t, map[string]string{
		"bad.go": `package demo

func Bad() {
	var count int = "x"
	_ = count
}
`,
	})
	typeResult, err := typeEditor.Validate(ctx, ValidationOptions{Kinds: []ValidationKind{ValidationTypecheck}})
	if err != nil {
		t.Fatal(err)
	}
	if typeResult.Passed || len(typeResult.Diagnostics) == 0 || typeResult.ResolutionMode != "typecheck" {
		t.Fatalf("expected typecheck validation failure, got %#v", typeResult)
	}
	if len(typeResult.AffectedPaths) != 1 || typeResult.AffectedPaths[0] != "bad.go" {
		t.Fatalf("unexpected validation paths: %#v", typeResult.AffectedPaths)
	}

	moduleEditor := newTestEditor(t, map[string]string{
		"go.mod": `module example.com/app
`,
		"main.go": `package app

import "example.com/app/core"

func Use() string {
	return core.Name()
}
`,
		"core/core.go": `package core

func Name() string {
	return "demo"
}
`,
	})
	moduleResult, err := moduleEditor.Validate(ctx, ValidationOptions{Kinds: []ValidationKind{ValidationTypecheck}})
	if err != nil {
		t.Fatal(err)
	}
	if !moduleResult.Passed || len(moduleResult.Diagnostics) != 0 {
		t.Fatalf("expected local module import validation to pass, got %#v", moduleResult)
	}
}

func TestChangeSetValidateUsesPendingOverlay(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"demo.go": `package demo

func Target() string {
	return "old"
}
`,
	})
	changes := ed.NewChangeSet()
	if err := changes.Apply(ctx, ReplaceFunction{
		Target: SymbolSelector{Name: "Target", Kind: SymbolFunction},
		Source: `func Target() string {
	return "new"
}
`,
	}); err != nil {
		t.Fatal(err)
	}
	result, err := changes.Validate(ctx, ValidationOptions{Kinds: []ValidationKind{ValidationParse, ValidationTypecheck}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Passed || len(result.Diagnostics) != 0 || result.ResolutionMode != "typecheck" {
		t.Fatalf("expected pending overlay validation to pass, got %#v", result)
	}
	diff, err := changes.Diff(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diff, `return "new"`) {
		t.Fatalf("expected pending diff to include replacement:\n%s", diff)
	}
	fragment, err := ed.ReadSymbol(ctx, SymbolSelector{Name: "Target", Kind: SymbolFunction})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(fragment.Source, `"new"`) {
		t.Fatalf("editor state changed before commit:\n%s", fragment.Source)
	}
}

func TestChangeSetValidateReportsPendingParseDiagnostics(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"demo.go": `package demo

func Target() string {
	return "old"
}
`,
	})
	changes := ed.NewChangeSet()
	changes.overlay["demo.go"] = []byte("package demo\n\nfunc Target( {\n}\n")
	changes.changed["demo.go"] = true

	result, err := changes.Validate(ctx, ValidationOptions{Kinds: []ValidationKind{ValidationParse}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Passed || len(result.Diagnostics) == 0 {
		t.Fatalf("expected pending parse validation failure, got %#v", result)
	}
	if len(result.AffectedPaths) != 1 || result.AffectedPaths[0] != "demo.go" {
		t.Fatalf("unexpected validation paths: %#v", result.AffectedPaths)
	}
}

func TestChangeSetValidateReportsPendingTypecheckDiagnostics(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"demo.go": `package demo

func Target() string {
	return "old"
}
`,
	})
	changes := ed.NewChangeSet()
	if err := changes.Apply(ctx, ReplaceFunction{
		Target: SymbolSelector{Name: "Target", Kind: SymbolFunction},
		Source: `func Target() string {
	return 1
}
`,
	}); err != nil {
		t.Fatal(err)
	}
	result, err := changes.Validate(ctx, ValidationOptions{Kinds: []ValidationKind{ValidationTypecheck}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Passed || len(result.Diagnostics) == 0 || result.ResolutionMode != "typecheck" {
		t.Fatalf("expected pending typecheck validation failure, got %#v", result)
	}
	fragment, err := ed.ReadSymbol(ctx, SymbolSelector{Name: "Target", Kind: SymbolFunction})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(fragment.Source, "return 1") {
		t.Fatalf("editor state changed before commit:\n%s", fragment.Source)
	}
}

func TestExtractGoFunctionAndMethod(t *testing.T) {
	ctx := context.Background()
	src := `package demo

import "strings"

type User struct {
	Name string
}

func Run(name string) string {
	return strings.TrimSpace(name)
}

func (u User) Label() string {
	return u.Name
}
`
	ed := newTestEditor(t, map[string]string{"demo.go": src})
	changes := ed.NewChangeSet()
	extractFuncStart := offsetOf(t, src, "return strings.TrimSpace(name)")
	extractFuncEnd := extractFuncStart + len("return strings.TrimSpace(name)")
	if err := changes.Apply(ctx, ExtractGoFunction{
		Path:            "demo.go",
		Range:           Range{Start: Position{Offset: extractFuncStart}, End: Position{Offset: extractFuncEnd}},
		Name:            "Normalize",
		Params:          "name string",
		Results:         "string",
		ReplaceWithCall: "return Normalize(name)",
	}); err != nil {
		t.Fatal(err)
	}
	afterFirst := string(mustFiles(t, changes, ctx)[0].After)
	extractMethodStart := offsetOf(t, afterFirst, "return u.Name")
	extractMethodEnd := extractMethodStart + len("return u.Name")
	if err := changes.Apply(ctx, ExtractGoMethod{
		Path:            "demo.go",
		Range:           Range{Start: Position{Offset: extractMethodStart}, End: Position{Offset: extractMethodEnd}},
		Receiver:        "u User",
		Name:            "NameValue",
		Results:         "string",
		ReplaceWithCall: "return u.NameValue()",
	}); err != nil {
		t.Fatal(err)
	}
	after := string(mustFiles(t, changes, ctx)[0].After)
	for _, want := range []string{
		"return Normalize(name)",
		"func Normalize(name string) string",
		"return strings.TrimSpace(name)",
		"return u.NameValue()",
		"func (u User) NameValue() string",
		"return u.Name",
	} {
		if !strings.Contains(after, want) {
			t.Fatalf("extract output missing %q:\n%s", want, after)
		}
	}
}

func TestMetricsIncludeSymbolPressure(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"hot.go": `package demo

func Hot() {}

func A() { Hot() }
func B() { Hot() }
func C() { Hot() }
func D() { Hot() }
func E() { Hot() }
`,
	})
	metrics, err := ed.Metrics(ctx, Scope{})
	if err != nil {
		t.Fatal(err)
	}
	var hot *SymbolMetrics
	for i := range metrics.Symbols {
		if metrics.Symbols[i].QualifiedName == "Hot" {
			hot = &metrics.Symbols[i]
			break
		}
	}
	if hot == nil {
		t.Fatalf("Hot symbol metrics not found: %#v", metrics.Symbols)
	}
	if hot.ReferenceCount < 5 || hot.CallFanIn < 5 || hot.PressureScore <= 0 {
		t.Fatalf("unexpected Hot metrics: %#v", *hot)
	}
}

func TestSuggestRefactoringsIncludesHighPressureSymbols(t *testing.T) {
	ctx := context.Background()
	ed := newTestEditor(t, map[string]string{
		"hot.go": `package demo

func Hot() {}

func A() { Hot() }
func B() { Hot() }
func C() { Hot() }
func D() { Hot() }
func E() { Hot() }
`,
	})
	proposals, err := ed.SuggestRefactorings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, proposal := range proposals {
		if proposal.Kind != RefactorSplitFunction {
			continue
		}
		for _, target := range proposal.Targets {
			if target.QualifiedName == "Hot" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("expected high-pressure symbol proposal for Hot: %#v", proposals)
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

import "strings"

func CreateUser(name string) string {
	return strings.TrimSpace(name)
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
	var storePackageID string
	for _, pkg := range packages.Packages {
		if pkg.Dir == "store" {
			storePackageID = pkg.ID
			break
		}
	}
	if storePackageID == "" {
		t.Fatalf("store package not found: %#v", packages.Packages)
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

	packageDirect, err := ed.ImportGraph(ctx, ImportQuery{PackageID: storePackageID, Direction: ImportDirectionDirect})
	if err != nil {
		t.Fatal(err)
	}
	gotImports := map[string]bool{}
	for _, imp := range packageDirect.DirectImports {
		gotImports[imp.Import] = true
	}
	if len(packageDirect.DirectImports) != 2 || !gotImports["example.com/demo/lib"] || !gotImports["strings"] {
		t.Fatalf("unexpected PackageID direct imports: %#v", packageDirect.DirectImports)
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

func TestGoBackendScopeUnitIDAndMaxFiles(t *testing.T) {
	ctx := context.Background()
	files := map[string]string{
		"a/a.go": `package a

import "example.com/demo/shared"

func A() {}
`,
		"b/b.go": `package b

import "strings"

func B() {}
`,
		"c/c.go": `package c

func C() {}
`,
	}
	ed := newTestEditor(t, files)

	unitOutline, err := ed.Outline(ctx, Scope{UnitID: "b#b"})
	if err != nil {
		t.Fatal(err)
	}
	if len(unitOutline.Documents) != 1 || unitOutline.Documents[0].URI != "b/b.go" {
		t.Fatalf("UnitID scope included unexpected documents: %#v", unitOutline.Documents)
	}
	if len(unitOutline.Symbols) != 1 || unitOutline.Symbols[0].Name != "B" || unitOutline.Symbols[0].UnitID != "b#b" {
		t.Fatalf("UnitID scope included unexpected symbols: %#v", unitOutline.Symbols)
	}

	limitedOutline, err := ed.Outline(ctx, Scope{MaxFiles: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(limitedOutline.Documents) != 1 || limitedOutline.Documents[0].URI != "a/a.go" {
		t.Fatalf("MaxFiles scope should limit documents to first sorted file: %#v", limitedOutline.Documents)
	}
	if len(limitedOutline.Symbols) != 1 || limitedOutline.Symbols[0].Location.URI != "a/a.go" {
		t.Fatalf("MaxFiles scope should limit symbols to first sorted file: %#v", limitedOutline.Symbols)
	}

	limitedPackages, err := ed.Packages(ctx, Scope{MaxFiles: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(limitedPackages.Packages) != 1 || limitedPackages.Packages[0].ID != "a#a" {
		t.Fatalf("MaxFiles scope should limit packages to first sorted file: %#v", limitedPackages.Packages)
	}

	limitedMetrics, err := ed.Metrics(ctx, Scope{MaxFiles: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(limitedMetrics.Units) != 1 || limitedMetrics.Units[0].UnitID != "a#a" {
		t.Fatalf("MaxFiles scope should limit metrics to first sorted file: %#v", limitedMetrics.Units)
	}

	limitedImports, err := ed.Imports(ctx, Scope{MaxFiles: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(limitedImports) != 1 || limitedImports[0].FromPath != "a/a.go" || limitedImports[0].Import != "example.com/demo/shared" {
		t.Fatalf("MaxFiles scope should limit imports to first sorted file: %#v", limitedImports)
	}
}

func TestNavigateLineColumnUsesByteColumnsWithUTF8(t *testing.T) {
	ctx := context.Background()
	src := `package utf8

func Caller() {
	println("éé"); Target()
}

func Target() {}
`
	ed := newTestEditor(t, map[string]string{"utf8.go": src})

	lineText := strings.Split(src, "\n")[3]
	byteColumn := strings.Index(lineText, "Target") + 1
	if byteColumn == 0 {
		t.Fatal("Target call not found on UTF-8 line")
	}
	runeColumn := len([]rune(lineText[:byteColumn-1])) + 1
	if byteColumn <= runeColumn {
		t.Fatalf("test line does not exercise UTF-8 byte-column skew: byte=%d rune=%d", byteColumn, runeColumn)
	}

	nav, err := ed.Navigate(ctx, PositionSelector{Path: "utf8.go", Line: 4, Column: byteColumn}, NavigationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(nav.Symbols) != 1 || nav.Symbols[0].Name != "Target" {
		t.Fatalf("unexpected line/column navigation result: %#v", nav)
	}

	callOffset := offsetOf(t, src, "Target()")
	offsetNav, err := ed.Navigate(ctx, PositionSelector{Path: "utf8.go", Offset: &callOffset}, NavigationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(offsetNav.Symbols) != 1 || offsetNav.Symbols[0].Name != "Target" {
		t.Fatalf("unexpected offset navigation result: %#v", offsetNav)
	}
	if nav.Target.Location.Range.Start.Offset != callOffset || offsetNav.Target.Location.Range.Start.Offset != callOffset {
		t.Fatalf("line/column and offset navigation resolved different starts: line/column=%d offset=%d want=%d", nav.Target.Location.Range.Start.Offset, offsetNav.Target.Location.Range.Start.Offset, callOffset)
	}
}

func TestReferencesAtRespectsPathScopeAndLimits(t *testing.T) {
	ctx := context.Background()
	files := map[string]string{
		"pkg/a.go": `package pkg

func Target() {}

func InScope() {
	Target()
	Target()
}
`,
		"pkg/b.go": `package pkg

func OutOfScope() {
	Target()
}
`,
		"pkg/a_test.go": `package pkg

func TestTarget() {
	Target()
}
`,
	}
	ed := newTestEditor(t, files)
	callOffset := offsetOf(t, files["pkg/a.go"], "Target()\n\tTarget")

	refs, err := ed.ReferencesAt(ctx, PositionSelector{Path: "pkg/a.go", Offset: &callOffset}, ReferenceOptions{
		Scope:              Scope{Path: "pkg/a.go"},
		IncludeDeclaration: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) == 0 {
		t.Fatal("expected scoped references")
	}
	for _, ref := range refs {
		if ref.Location.URI != "pkg/a.go" {
			t.Fatalf("out-of-scope reference returned: %#v", refs)
		}
		if ref.Kind == OccurrenceDeclaration {
			t.Fatalf("declaration should have been excluded: %#v", refs)
		}
	}

	withDecl, err := ed.ReferencesAt(ctx, PositionSelector{Path: "pkg/a.go", Offset: &callOffset}, ReferenceOptions{
		Scope:              Scope{Path: "pkg/a.go"},
		IncludeDeclaration: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	foundDecl := false
	for _, ref := range withDecl {
		if ref.Kind == OccurrenceDeclaration {
			foundDecl = true
		}
		if ref.Location.URI != "pkg/a.go" {
			t.Fatalf("out-of-scope reference returned with declaration: %#v", withDecl)
		}
	}
	if !foundDecl {
		t.Fatalf("expected declaration when IncludeDeclaration is true: %#v", withDecl)
	}

	limited, err := ed.ReferencesAt(ctx, PositionSelector{Path: "pkg/a.go", Offset: &callOffset}, ReferenceOptions{
		Scope:              Scope{Path: "pkg/a.go"},
		IncludeDeclaration: false,
		MaxResults:         1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 1 {
		t.Fatalf("expected MaxResults to limit references to 1, got %#v", limited)
	}

	withoutTests, err := ed.ReferencesAt(ctx, PositionSelector{Path: "pkg/a.go", Offset: &callOffset}, ReferenceOptions{
		Scope:              Scope{Path: "pkg"},
		IncludeDeclaration: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range withoutTests {
		if strings.HasSuffix(ref.Location.URI, "_test.go") {
			t.Fatalf("test reference returned when IncludeTests is false: %#v", withoutTests)
		}
	}

	withTests, err := ed.ReferencesAt(ctx, PositionSelector{Path: "pkg/a.go", Offset: &callOffset}, ReferenceOptions{
		Scope:              Scope{Path: "pkg", IncludeTests: true},
		IncludeDeclaration: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	foundTestRef := false
	for _, ref := range withTests {
		if strings.HasSuffix(ref.Location.URI, "_test.go") {
			foundTestRef = true
		}
	}
	if !foundTestRef {
		t.Fatalf("expected test reference when IncludeTests is true: %#v", withTests)
	}
}

func TestReferencesAtRespectsUnitIDScope(t *testing.T) {
	ctx := context.Background()
	lang := LanguageID("ref")
	ed := newTestEditorWithOptions(t, map[string]string{
		"in.ref":  "Demo local\n",
		"out.ref": "remote\n",
	}, WithBackend(referenceBackend{}), WithLanguage(lang))
	offset := 0

	refs, err := ed.ReferencesAt(ctx, PositionSelector{Path: "in.ref", Offset: &offset}, ReferenceOptions{
		Scope:              Scope{UnitID: "unit/in", Language: lang},
		IncludeDeclaration: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) == 0 {
		t.Fatal("expected unit-scoped references")
	}
	for _, ref := range refs {
		if ref.Location.URI != "in.ref" {
			t.Fatalf("out-of-unit reference returned: %#v", refs)
		}
		if ref.Kind == OccurrenceDeclaration {
			t.Fatalf("declaration should have been excluded: %#v", refs)
		}
	}

	limited, err := ed.ReferencesAt(ctx, PositionSelector{Path: "in.ref", Offset: &offset}, ReferenceOptions{
		Scope:              Scope{UnitID: "unit/in", Language: lang},
		IncludeDeclaration: true,
		MaxResults:         1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 1 {
		t.Fatalf("expected MaxResults to limit unit-scoped references to 1, got %#v", limited)
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

func changedFilesByPath(files []ChangedFile) map[string]ChangedFile {
	out := map[string]ChangedFile{}
	for _, file := range files {
		out[file.Path] = file
	}
	return out
}

func mustFiles(t *testing.T, changes *ChangeSet, ctx context.Context) []ChangedFile {
	t.Helper()
	files, err := changes.Files(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func onlySymbol(t *testing.T, idx *core.Index, name string, kind SymbolKind) Symbol {
	t.Helper()
	var out []Symbol
	for _, sym := range idx.Symbols {
		if sym.Name == name && sym.Kind == kind {
			out = append(out, sym)
		}
	}
	if len(out) != 1 {
		t.Fatalf("expected one %s %q, got %#v", kind, name, out)
	}
	return out[0]
}

func occurrencesByKind(occurrences []Occurrence) map[OccurrenceKind]int {
	out := map[OccurrenceKind]int{}
	for _, occ := range occurrences {
		out[occ.Kind]++
	}
	return out
}

func countOccurrenceKind(occurrences []Occurrence, id SymbolID, kind OccurrenceKind) int {
	n := 0
	for _, occ := range occurrences {
		if occ.SymbolID == id && occ.Kind == kind {
			n++
		}
	}
	return n
}

func TestGoParserPackagesOnlyLiveInGoBackend(t *testing.T) {
	files := productionGoFiles(t)
	for _, file := range files {
		if strings.HasPrefix(file, "internal/lang/goast/") {
			continue
		}
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{`"go/ast"`, `"go/parser"`, `"go/token"`, `"go/format"`, `"go/types"`, `"golang.org/x/tools/go/packages"`} {
			if strings.Contains(string(src), forbidden) {
				t.Fatalf("%s imports %s; Go parser/type packages must stay in language-specific backends", file, forbidden)
			}
		}
	}
}

func TestCoreDoesNotUseHostOSOrProcessExecution(t *testing.T) {
	for _, file := range productionGoFiles(t) {
		if strings.HasPrefix(file, "adapter/") || strings.HasPrefix(file, "cmd/") || strings.HasPrefix(file, "internal/lang/") {
			continue
		}
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{`"os"`, `"os/exec"`, "exec.Command", "git "} {
			if strings.Contains(string(src), forbidden) {
				t.Fatalf("%s contains forbidden core dependency or command marker %q", file, forbidden)
			}
		}
	}
}

func productionGoFiles(t *testing.T) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch p {
			case ".git", ".agents", "vendor":
				return filepath.SkipDir
			default:
				return nil
			}
		}
		if filepath.Ext(p) == ".go" && !strings.HasSuffix(p, "_test.go") {
			files = append(files, filepath.ToSlash(p))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
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
			"main.go": []byte(`package main

func main() {}

func unusedHelper(flag bool, a int, b int, c int, d int) {}
`),
		},
	}
	ed, err := NewEditor(".", WithSource(source), WithLanguage(Go))
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
	metrics, err := ed.Metrics(ctx, Scope{})
	if err != nil {
		t.Fatal(err)
	}
	if len(metrics.Units) != 1 || metrics.Units[0].UnitID != "main" {
		t.Fatalf("expected source-backed metrics, got %#v", metrics)
	}
	proposals, err := ed.SuggestRefactorings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[RefactorKind]bool{}
	for _, proposal := range proposals {
		kinds[proposal.Kind] = true
	}
	if !kinds[RefactorDeleteSymbol] || !kinds[RefactorIntroduceConfig] || !kinds[RefactorReplaceFlagArgument] {
		t.Fatalf("expected source-backed refactor suggestions, got %#v", proposals)
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

func (s mapSource) ReadFile(ctx context.Context, path string) ([]byte, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
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
	ed, err := NewEditor(".", all...)
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
		Capabilities:   []CapabilitySupport{{Capability: CapabilityStaticAnalysis, Level: CapabilityBasic}},
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
		src, err := snapshot.ReadFile(ctx, file)
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

type referenceBackend struct{}

func (referenceBackend) Spec() BackendSpec {
	return BackendSpec{
		Language:       LanguageID("ref"),
		Name:           "ref",
		FileExtensions: []string{".ref"},
		Capabilities:   []CapabilitySupport{{Capability: CapabilityStaticAnalysis, Level: CapabilityBasic}},
		ResolutionMode: "ref",
	}
}

func (referenceBackend) Index(ctx context.Context, snapshot core.Snapshot, scope Scope) (*core.Index, error) {
	idx := core.NewIndex()
	files, err := snapshot.ListFiles(ctx, scope)
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		if filepath.Ext(file) != ".ref" {
			continue
		}
		src, err := snapshot.ReadFile(ctx, file)
		if err != nil {
			return nil, err
		}
		unit := "unit/out"
		if file == "in.ref" {
			unit = "unit/in"
		}
		idx.Documents = append(idx.Documents, Document{URI: file, Language: LanguageID("ref"), UnitID: unit})
		idx.FileUnits[file] = unit
		idx.UnitFiles[unit] = append(idx.UnitFiles[unit], file)
		if file == "in.ref" {
			sym := Symbol{
				ID:             SymbolID("ref:Demo"),
				Language:       LanguageID("ref"),
				Kind:           SymbolClass,
				Name:           "Demo",
				QualifiedName:  "Demo",
				UnitID:         unit,
				Location:       Location{URI: file, Range: Range{Start: Position{Offset: 0}, End: Position{Offset: len(src)}}},
				SelectionRange: Range{Start: Position{Offset: 0}, End: Position{Offset: len("Demo")}},
				Backend:        BackendInfo{Language: LanguageID("ref"), Name: "ref", ResolutionMode: "ref", Complete: true},
			}
			idx.Symbols = append(idx.Symbols, sym)
			idx.ByID[sym.ID] = sym
			idx.ByName[sym.Name] = append(idx.ByName[sym.Name], sym)
			idx.Occurrences = append(idx.Occurrences, Occurrence{
				SymbolID: sym.ID,
				Kind:     OccurrenceDeclaration,
				Name:     sym.Name,
				Location: Location{URI: file, Range: sym.SelectionRange},
			})
			localOffset := strings.Index(string(src), "local")
			if localOffset >= 0 {
				idx.Occurrences = append(idx.Occurrences, Occurrence{
					SymbolID: sym.ID,
					Kind:     OccurrenceReference,
					Name:     sym.Name,
					Location: Location{URI: file, Range: Range{Start: Position{Offset: localOffset}, End: Position{Offset: localOffset + len("local")}}},
				})
			}
		}
		if file == "out.ref" {
			idx.Occurrences = append(idx.Occurrences, Occurrence{
				SymbolID: SymbolID("ref:Demo"),
				Kind:     OccurrenceReference,
				Name:     "Demo",
				Location: Location{URI: file, Range: Range{Start: Position{Offset: 0}, End: Position{Offset: len(src)}}},
			})
		}
	}
	core.SortSymbols(idx.Symbols)
	core.SortOccurrences(idx.Occurrences)
	return idx, nil
}

func (referenceBackend) CompileEdit(context.Context, core.Snapshot, Operation) ([]FileEdit, error) {
	return nil, errors.New("reference backend does not support edits")
}

func (referenceBackend) Format(_ context.Context, _ string, src []byte) ([]byte, error) {
	return src, nil
}

func (referenceBackend) Suggest(context.Context, core.Snapshot, Scope) ([]Proposal, error) {
	return nil, nil
}
