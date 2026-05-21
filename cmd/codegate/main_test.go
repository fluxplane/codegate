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
	if !strings.Contains(got, `"language": "go"`) || !strings.Contains(got, `"capability": "lookup"`) {
		t.Fatalf("unexpected capabilities output:\n%s", got)
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
