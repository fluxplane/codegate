package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodegateAssessCommand(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	writeFile(t, root, "demo.go", `package demo

func Target() string {
	return "ok"
}
`)

	var out bytes.Buffer
	a := &app{out: &out, err: &bytes.Buffer{}}
	cmd := a.rootCommand()
	cmd.SetContext(context.Background())
	cmd.SetArgs([]string{"--root", root, "assess", "--suggestions", "2"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, `"validation"`) || !strings.Contains(got, `"passed": true`) || !strings.Contains(got, `"suggestions"`) {
		t.Fatalf("unexpected assess output:\n%s", got)
	}
}

func TestCodegateAssessGateCommand(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	writeFile(t, root, "demo.go", `package demo

func Target() string {
	return "ok"
}
`)

	var out bytes.Buffer
	a := &app{out: &out, err: &bytes.Buffer{}}
	cmd := a.rootCommand()
	cmd.SetContext(context.Background())
	cmd.SetArgs([]string{"--root", root, "assess", "--gate", "architecture", "--suggestions", "1"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, `"provider_score_model": "go-architecture-v0"`) || !strings.Contains(got, `"architecture"`) {
		t.Fatalf("unexpected gated assess output:\n%s", got)
	}
}

func TestCodegateAssessRulesCommand(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	writeFile(t, root, "domain/domain.go", `package domain

import "example.com/demo/infra"

func UseInfra() string {
	return infra.Name
}
`)
	writeFile(t, root, "infra/infra.go", `package infra

const Name = "infra"
`)
	rulesPath := filepath.Join(root, "codegate.rules.json")
	if err := os.WriteFile(rulesPath, []byte(`{
  "imports": [
    {
      "from": "domain",
      "to": "example.com/demo/infra",
      "action": "deny",
      "reason": "domain must not depend on infra"
    }
  ]
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	a := &app{out: &out, err: &bytes.Buffer{}}
	cmd := a.rootCommand()
	cmd.SetContext(context.Background())
	cmd.SetArgs([]string{"--root", root, "assess", "--gate", "architecture", "--rules", rulesPath})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, `"architecture_denied_import"`) || !strings.Contains(got, `"domain must not depend on infra"`) {
		t.Fatalf("unexpected rules assess output:\n%s", got)
	}
}

func TestCodegateAssessFailOnBoundary(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	writeFile(t, root, "domain/domain.go", `package domain

import "example.com/demo/infra"

func UseInfra() string {
	return infra.Name
}
`)
	writeFile(t, root, "infra/infra.go", `package infra

const Name = "infra"
`)
	rulesPath := filepath.Join(root, "codegate.rules.json")
	writeRulesFile(t, rulesPath, `{
  "imports": [
    {"from": "domain", "to": "example.com/demo/infra", "action": "deny", "reason": "domain must not depend on infra"}
  ]
}`)

	var out bytes.Buffer
	a := &app{out: &out, err: &bytes.Buffer{}}
	cmd := a.rootCommand()
	cmd.SetContext(context.Background())
	cmd.SetArgs([]string{"--root", root, "assess", "--gate", "architecture", "--rules", rulesPath, "--fail-on", "boundary"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected boundary failure")
	}
	if got := out.String(); !strings.Contains(got, `"architecture_denied_import"`) {
		t.Fatalf("expected JSON report before failure, got:\n%s", got)
	}
}

func TestCodegateAssessFailOnEffects(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	writeFile(t, root, "core/core.go", `package core

import "os"

func Token() string {
	return os.Getenv("TOKEN")
}
`)
	rulesPath := filepath.Join(root, "codegate.rules.json")
	writeRulesFile(t, rulesPath, `{
  "layers": [{"name": "domain", "prefixes": ["core"]}],
  "effects": [
    {
      "name": "host_io",
      "scope": {"layers": ["domain"]},
      "imports": ["os"],
      "calls": [{"import": "os", "symbol": "Getenv"}],
      "reason": "domain must not access host IO directly"
    }
  ]
}`)

	var out bytes.Buffer
	a := &app{out: &out, err: &bytes.Buffer{}}
	cmd := a.rootCommand()
	cmd.SetContext(context.Background())
	cmd.SetArgs([]string{"--root", root, "assess", "--gate", "architecture", "--rules", rulesPath, "--fail-on", "effects"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected effects failure")
	}
	if got := out.String(); !strings.Contains(got, `"architecture_host_io"`) {
		t.Fatalf("expected JSON effect report before failure, got:\n%s", got)
	}
}

func TestCodegateAssessFailOnUnknown(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	writeFile(t, root, "experimental/foo/foo.go", `package foo

const Name = "foo"
`)
	rulesPath := filepath.Join(root, "codegate.rules.json")
	writeRulesFile(t, rulesPath, `{
  "layers": [{"name": "core", "prefixes": ["core"]}]
}`)

	var out bytes.Buffer
	a := &app{out: &out, err: &bytes.Buffer{}}
	cmd := a.rootCommand()
	cmd.SetContext(context.Background())
	cmd.SetArgs([]string{"--root", root, "assess", "--gate", "architecture", "--rules", rulesPath, "--fail-on", "unknown"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected unknown-package failure")
	}
	if got := out.String(); !strings.Contains(got, `"architecture_unknown_package"`) {
		t.Fatalf("expected JSON unknown-package report before failure, got:\n%s", got)
	}
}

func TestCodegateAssessFailOnAllIgnoresReviewedFanOut(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	writeFile(t, root, "core/hub/hub.go", `package hub

import (
	"example.com/demo/core/a"
	"example.com/demo/core/b"
)

func Names() []string {
	return []string{a.Name, b.Name}
}
`)
	writeFile(t, root, "core/a/a.go", `package a

const Name = "a"
`)
	writeFile(t, root, "core/b/b.go", `package b

const Name = "b"
`)
	rulesPath := filepath.Join(root, "codegate.rules.json")
	writeRulesFile(t, rulesPath, `{
  "layers": [{"name": "core", "prefixes": ["core"]}],
  "dependencies": [{"from_layer": "core", "to_layer": "core"}],
  "coupling": {
    "fan_out_threshold": 1,
    "layers": ["core"],
    "reviewed_fan_out": [
      {"package": "core/hub", "reason": "hub intentionally aggregates core packages"}
    ]
  }
}`)

	var out bytes.Buffer
	a := &app{out: &out, err: &bytes.Buffer{}}
	cmd := a.rootCommand()
	cmd.SetContext(context.Background())
	cmd.SetArgs([]string{"--root", root, "assess", "--gate", "architecture", "--rules", rulesPath, "--fail-on", "all"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("reviewed fan-out should not fail selected gates: %v\n%s", err, out.String())
	}
	if got := out.String(); !strings.Contains(got, `"architecture_fan_out"`) || !strings.Contains(got, `"allowed": true`) {
		t.Fatalf("expected reviewed fan-out finding, got:\n%s", got)
	}
}

func TestCodegateAssessRejectsUnknownGate(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	writeFile(t, root, "demo.go", "package demo\n")

	a := &app{out: &bytes.Buffer{}, err: &bytes.Buffer{}}
	cmd := a.rootCommand()
	cmd.SetContext(context.Background())
	cmd.SetArgs([]string{"--root", root, "assess", "--gate", "unknown"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected unknown gate to fail")
	}
}

func TestCodegateLookupCommand(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	writeFile(t, root, "demo.go", `package demo

func Target() string {
	return "ok"
}
`)

	var out bytes.Buffer
	a := &app{out: &out, err: &bytes.Buffer{}}
	cmd := a.rootCommand()
	cmd.SetContext(context.Background())
	cmd.SetArgs([]string{"--root", root, "lookup", "--name", "Target", "--kind", "function"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, `"Name": "Target"`) || !strings.Contains(got, `"QualifiedName": "Target"`) {
		t.Fatalf("unexpected lookup output:\n%s", got)
	}
}

func TestCodegateCapabilitiesCommand(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24\n")
	writeFile(t, root, "demo.go", "package demo\n")

	var out bytes.Buffer
	a := &app{out: &out, err: &bytes.Buffer{}}
	cmd := a.rootCommand()
	cmd.SetContext(context.Background())
	cmd.SetArgs([]string{"--root", root, "capabilities"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, `"language": "go"`) || !strings.Contains(got, `"language": "markdown"`) || !strings.Contains(got, `"capability": "lookup"`) {
		t.Fatalf("unexpected capabilities output:\n%s", got)
	}
}

func TestCodegateMarkdownCommands(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "README.md", `# Demo

See [Missing](#missing).

### Jumped
`)

	var assessOut bytes.Buffer
	a := &app{out: &assessOut, err: &bytes.Buffer{}}
	cmd := a.rootCommand()
	cmd.SetContext(context.Background())
	cmd.SetArgs([]string{"--root", root, "--language", "markdown", "assess", "--gate", "maintainability"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := assessOut.String()
	if !strings.Contains(got, `"provider_score_model": "markdown-structure-v0"`) || !strings.Contains(got, `"markdown_heading_level_jump"`) {
		t.Fatalf("unexpected markdown assess output:\n%s", got)
	}

	var lookupOut bytes.Buffer
	a = &app{out: &lookupOut, err: &bytes.Buffer{}}
	cmd = a.rootCommand()
	cmd.SetContext(context.Background())
	cmd.SetArgs([]string{"--root", root, "--language", "markdown", "lookup", "--name", "Jumped", "--kind", "namespace"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got = lookupOut.String()
	if !strings.Contains(got, `"Name": "Jumped"`) || !strings.Contains(got, `"Language": "markdown"`) {
		t.Fatalf("unexpected markdown lookup output:\n%s", got)
	}
}

func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeRulesFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
