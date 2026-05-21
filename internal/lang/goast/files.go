package goast

import (
	"bytes"
	"context"
	"go/build"
	"go/build/constraint"
	"path"
	"runtime"
	"sort"
	"strings"

	"github.com/fluxplane/codegate/internal/core"
)

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
		p = core.CleanPath(p)
		if !isGoSourcePath(p, scope) || !goFileNameMatchesBuild(p) {
			continue
		}
		src, err := snapshot.ReadFile(ctx, p)
		if err != nil {
			return nil, err
		}
		if !goBuildConstraintsMatch(src) {
			continue
		}
		out = append(out, p)
	}
	sort.Strings(out)
	return out, nil
}

func isGoSourcePath(p string, scope Scope) bool {
	return strings.HasSuffix(p, ".go") && (scope.IncludeTests || !core.HasTestPath(p))
}

func goFileNameMatchesBuild(p string) bool {
	name := strings.TrimSuffix(path.Base(p), ".go")
	parts := strings.Split(name, "_")
	if len(parts) < 2 {
		return true
	}
	last := parts[len(parts)-1]
	if knownGOARCH[last] {
		if last != runtime.GOARCH {
			return false
		}
		if len(parts) >= 3 {
			if goos := parts[len(parts)-2]; knownGOOS[goos] && goos != runtime.GOOS {
				return false
			}
		}
		return true
	}
	if knownGOOS[last] {
		return last == runtime.GOOS
	}
	return true
}

func goBuildConstraintsMatch(src []byte) bool {
	var exprs []constraint.Expr
	for _, line := range leadingBuildConstraintLines(src) {
		expr, err := constraint.Parse(line)
		if err != nil {
			return true
		}
		exprs = append(exprs, expr)
	}
	for _, expr := range exprs {
		if !expr.Eval(goBuildTagActive) {
			return false
		}
	}
	return true
}

func leadingBuildConstraintLines(src []byte) []string {
	var lines []string
	for _, raw := range bytes.Split(src, []byte{'\n'}) {
		line := strings.TrimSpace(string(raw))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "//go:build ") || strings.HasPrefix(line, "// +build ") {
			lines = append(lines, line)
			continue
		}
		if strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*") || strings.HasPrefix(line, "*") {
			continue
		}
		break
	}
	return lines
}

func goBuildTagActive(tag string) bool {
	switch tag {
	case runtime.GOOS, runtime.GOARCH, "gc":
		return true
	case "cgo":
		return build.Default.CgoEnabled
	case "unix":
		return unixGOOS[runtime.GOOS]
	default:
		for _, releaseTag := range build.Default.ReleaseTags {
			if tag == releaseTag {
				return true
			}
		}
		return false
	}
}

var knownGOOS = map[string]bool{
	"aix": true, "android": true, "darwin": true, "dragonfly": true, "freebsd": true,
	"hurd": true, "illumos": true, "ios": true, "js": true, "linux": true,
	"netbsd": true, "openbsd": true, "plan9": true, "solaris": true, "wasip1": true,
	"windows": true,
}

var knownGOARCH = map[string]bool{
	"386": true, "amd64": true, "amd64p32": true, "arm": true, "armbe": true,
	"arm64": true, "arm64be": true, "loong64": true, "mips": true, "mipsle": true,
	"mips64": true, "mips64le": true, "mips64p32": true, "mips64p32le": true,
	"ppc": true, "ppc64": true, "ppc64le": true, "riscv": true, "riscv64": true,
	"s390": true, "s390x": true, "sparc": true, "sparc64": true, "wasm": true,
}

var unixGOOS = map[string]bool{
	"aix": true, "android": true, "darwin": true, "dragonfly": true, "freebsd": true,
	"hurd": true, "illumos": true, "ios": true, "linux": true, "netbsd": true,
	"openbsd": true, "solaris": true,
}
