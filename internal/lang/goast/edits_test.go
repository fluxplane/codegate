package goast

import (
	"context"
	"strings"
	"testing"
)

type testSnapshot map[string]string

func (s testSnapshot) ListFiles(ctx context.Context, scope Scope) ([]string, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	files := make([]string, 0, len(s))
	for p := range s {
		files = append(files, p)
	}
	return files, nil
}

func (s testSnapshot) ReadFile(ctx context.Context, path string) ([]byte, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return []byte(s[path]), nil
}

func TestCompileRenameGoModulePath(t *testing.T) {
	ctx := context.Background()
	snapshot := testSnapshot{
		"go.mod": "module github.com/acme/old\n\ngo 1.24\n",
		"a.go": `package demo

import (
	alias "github.com/acme/old/pkg" // keep
	"github.com/acme/old/other"
	"github.com/elsewhere/module"
)

const fixture = ` + "`" + `require github.com/acme/old v0.0.0
replace github.com/acme/old => ../old
` + "`" + `
`,
		"a_test.go": `package demo

import "github.com/acme/old/testkit"
`,
		"vendor/github.com/acme/old/v.go": `package vendor

import "github.com/acme/old/ignored"
`,
	}
	edits, err := New().CompileEdit(ctx, snapshot, RenameGoModulePath{
		OldPath: "github.com/acme/old",
		NewPath: "github.com/acme/new",
	})
	if err != nil {
		t.Fatal(err)
	}
	byPath := fileEditsByPath(edits)
	if len(byPath) != 3 {
		t.Fatalf("expected go.mod, a.go, and a_test.go edits, got %#v", byPath)
	}
	if got := replacementText(byPath["go.mod"]); got != "github.com/acme/new" {
		t.Fatalf("unexpected go.mod replacement %q", got)
	}
	if got := replacementText(byPath["a.go"]); !strings.Contains(got, "github.com/acme/new/pkg") || !strings.Contains(got, "github.com/acme/new/other") {
		t.Fatalf("unexpected a.go replacements %q", got)
	}
	if got := replacementText(byPath["a.go"]); !strings.Contains(got, "github.com/acme/new v0.0.0") || !strings.Contains(got, "replace github.com/acme/new") {
		t.Fatalf("string literal was not rewritten: %q", got)
	}
	if got := replacementText(byPath["a_test.go"]); got != `"github.com/acme/new/testkit"` {
		t.Fatalf("unexpected test import replacement %q", got)
	}
	if _, ok := byPath["vendor/github.com/acme/old/v.go"]; ok {
		t.Fatalf("vendor file should not be edited: %#v", byPath)
	}
}

func TestCompileRenameGoModulePathAllowsLocalNames(t *testing.T) {
	ctx := context.Background()
	_, err := New().CompileEdit(ctx, testSnapshot{
		"go.mod": "module localmod\n\ngo 1.24\n",
		"a.go": `package demo

import "localmod/pkg"
`,
	}, RenameGoModulePath{OldPath: "localmod", NewPath: "newlocalmod"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCompileRenameGoModulePathRewritesStringOnlyFixtures(t *testing.T) {
	ctx := context.Background()
	edits, err := New().CompileEdit(ctx, testSnapshot{
		"go.mod": "module github.com/acme/old\n\ngo 1.24\n",
		"release_validation_test.go": `package demo

const consumerGoMod = ` + "`" + `module example.com/consumer

require github.com/acme/old v0.0.0

replace github.com/acme/old => ../repo
` + "`" + `
`,
	}, RenameGoModulePath{OldPath: "github.com/acme/old", NewPath: "github.com/acme/new"})
	if err != nil {
		t.Fatal(err)
	}
	byPath := fileEditsByPath(edits)
	got := replacementText(byPath["release_validation_test.go"])
	if !strings.Contains(got, "require github.com/acme/new") || !strings.Contains(got, "replace github.com/acme/new") {
		t.Fatalf("string-only fixture was not rewritten: %q", got)
	}
}

func TestRewriteModulePathReferencesUsesPathBoundaries(t *testing.T) {
	got, ok := rewriteModulePathReferences(
		`require github.com/acme/old v0.0.0
replace github.com/acme/old => ../repo
import github.com/acme/old/pkg
do not touch github.com/acme/oldest
do not touch https://github.com/acme/old
do not touch prefixgithub.com/acme/old`,
		"github.com/acme/old",
		"github.com/acme/new",
	)
	if !ok {
		t.Fatal("expected module references to be rewritten")
	}
	for _, want := range []string{
		"require github.com/acme/new v0.0.0",
		"replace github.com/acme/new => ../repo",
		"import github.com/acme/new/pkg",
		"github.com/acme/oldest",
		"https://github.com/acme/old",
		"prefixgithub.com/acme/old",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in rewritten string:\n%s", want, got)
		}
	}
	if strings.Contains(got, "github.com/acme/newest") || strings.Contains(got, "https://github.com/acme/new") {
		t.Fatalf("rewrote non-module substrings:\n%s", got)
	}
}

func TestImportLocalNameHandlesSemanticImportVersions(t *testing.T) {
	for _, tc := range []struct {
		path  string
		alias string
		want  string
	}{
		{path: "github.com/labstack/echo/v4", want: "echo"},
		{path: "gopkg.in/yaml.v3", want: "yaml"},
		{path: "github.com/babelforce/rtvbp-go", want: "rtvbp"},
		{path: "github.com/hashicorp/go-multierror", want: "multierror"},
		{path: "github.com/testcontainers/testcontainers-go", want: "testcontainers"},
		{path: "github.com/acme/pkg", want: "pkg"},
		{path: "github.com/acme/pkg/v1", want: "v1"},
		{path: "github.com/acme/pkg/v4", alias: "custom", want: "custom"},
		{path: "github.com/acme/pkg/v4", alias: "_", want: ""},
	} {
		if got := importLocalName(tc.path, tc.alias); got != tc.want {
			t.Fatalf("importLocalName(%q, %q) = %q, want %q", tc.path, tc.alias, got, tc.want)
		}
	}
}

func TestValidateUsesGeneratedFilesForTypecheckContext(t *testing.T) {
	ctx := context.Background()
	result, err := New().Validate(ctx, testSnapshot{
		"go.mod": "module example.com/demo\n\ngo 1.24\n",
		"api.go": `package demo

func UseGenerated(v GeneratedType) string {
	return v.Name
}
`,
		"api.gen.go": `// Code generated by test. DO NOT EDIT.
package demo

type GeneratedType struct {
	Name string
}
`,
	}, ValidationOptions{
		Scope: Scope{Language: Go},
		Kinds: []ValidationKind{ValidationTypecheck},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Passed {
		t.Fatalf("expected generated file to be available as typecheck context, got %#v", result.Diagnostics)
	}
	for _, path := range result.AffectedPaths {
		if path == "api.gen.go" {
			t.Fatalf("generated file should not be reported as selected by default: %#v", result.AffectedPaths)
		}
	}
}

func TestValidateSuppressesUnresolvedExternalSelectorQualifiers(t *testing.T) {
	ctx := context.Background()
	result, err := New().Validate(ctx, testSnapshot{
		"go.mod": "module example.com/demo\n\ngo 1.24\n",
		"api.go": `package demo

import "github.com/acme/weird-name"

func UseExternal(v actualpkg.Type) string {
	return actualpkg.Render(v)
}
`,
	}, ValidationOptions{
		Scope: Scope{Language: Go},
		Kinds: []ValidationKind{ValidationTypecheck},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Passed {
		t.Fatalf("external selector qualifier diagnostics should be suppressed in best-effort typecheck, got %#v", result.Diagnostics)
	}
}

func TestCompileRenameGoModulePathRejectsMismatchAndCollisions(t *testing.T) {
	ctx := context.Background()
	_, err := New().CompileEdit(ctx, testSnapshot{
		"go.mod": "module github.com/acme/current\n\ngo 1.24\n",
		"a.go":   "package demo\n",
	}, RenameGoModulePath{OldPath: "github.com/acme/old", NewPath: "github.com/acme/new"})
	if err == nil {
		t.Fatal("expected mismatched module path to fail")
	}

	_, err = New().CompileEdit(ctx, testSnapshot{
		"go.mod": "module github.com/acme/old\n\ngo 1.24\n",
		"a.go": `package demo

import (
	"github.com/acme/new/pkg"
	"github.com/acme/old/pkg"
)
`,
	}, RenameGoModulePath{OldPath: "github.com/acme/old", NewPath: "github.com/acme/new"})
	if err == nil {
		t.Fatal("expected duplicate import collision to fail")
	}
}

func fileEditsByPath(edits []FileEdit) map[string]FileEdit {
	out := map[string]FileEdit{}
	for _, edit := range edits {
		out[edit.Path] = edit
	}
	return out
}

func replacementText(edit FileEdit) string {
	var b strings.Builder
	for _, textEdit := range edit.Edits {
		b.WriteString(textEdit.Replacement)
	}
	return b.String()
}
