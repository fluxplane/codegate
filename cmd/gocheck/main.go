package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"

	"github.com/fluxplane/codegate"
	"github.com/fluxplane/codegate/internal/lang/goast"
)

func main() {
	root := flag.String("root", ".", "workspace root to analyze")
	includeTests := flag.Bool("tests", false, "include Go test files")
	maxProposals := flag.Int("proposals", 10, "maximum refactor proposals to print")
	flag.Parse()

	if err := run(context.Background(), *root, *includeTests, *maxProposals); err != nil {
		fmt.Fprintf(os.Stderr, "gocheck: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, root string, includeTests bool, maxProposals int) error {
	source := dirSource{root: root, fsys: os.DirFS(root)}
	ed, err := codegate.NewEditor(".", codegate.WithSource(source), codegate.WithLanguage(codegate.Go))
	if err != nil {
		return err
	}
	scope := codegate.Scope{IncludeTests: includeTests}

	packages, err := ed.Packages(ctx, scope)
	if err != nil {
		return err
	}
	symbols, err := ed.FindSymbols(ctx, codegate.SymbolSelector{Language: codegate.Go, IncludeTests: &includeTests})
	if err != nil {
		return err
	}
	imports, err := ed.Imports(ctx, scope)
	if err != nil {
		return err
	}
	metrics, err := ed.Metrics(ctx, scope)
	if err != nil {
		return err
	}
	validation, err := ed.Validate(ctx, codegate.ValidationOptions{Scope: scope, Kinds: []codegate.ValidationKind{codegate.ValidationParse, codegate.ValidationTypecheck}})
	if err != nil {
		return err
	}
	proposals, err := ed.SuggestRefactorings(ctx, codegate.WithSuggestScope(scope))
	if err != nil {
		return err
	}
	goIndex, err := goast.New().Index(ctx, source, scope)
	if err != nil {
		return err
	}

	goFiles, err := countGoFiles(source.fsys, includeTests)
	if err != nil {
		return err
	}
	limitations, err := goast.CallSiteLimitations(ctx, source, scope)
	if err != nil {
		return err
	}

	fmt.Printf("root: %s\n", root)
	fmt.Printf("go files: %d\n", goFiles)
	fmt.Printf("packages: %d\n", len(packages.Packages))
	fmt.Printf("symbols: %d\n", len(symbols))
	fmt.Printf("imports: %d\n", len(imports))
	fmt.Printf("diagnostics: %d\n", len(packages.Diagnostics)+len(metrics.Diagnostics))
	fmt.Printf("validation: passed=%t mode=%s diagnostics=%d files=%d\n", validation.Passed, validation.ResolutionMode, len(validation.Diagnostics), len(validation.AffectedPaths))
	printValidationDiagnostics(validation.Diagnostics, 8)
	fmt.Printf("limitations: ast_only=true selector_calls=%d complex_callees=%d variadic_calls=%d\n", limitations.SelectorCalls, limitations.ComplexCallees, limitations.VariadicCalls)
	fmt.Printf("occurrences: %s\n", formatOccurrenceCounts(occurrenceCounts(goIndex.Occurrences)))
	fmt.Println()

	printTopMetrics(metrics.Units, 8)
	fmt.Println()
	printProposals(proposals, maxProposals)
	return nil
}

type dirSource struct {
	root string
	fsys fs.FS
}

func (s dirSource) WorkspaceRoot() string {
	return s.root
}

func (s dirSource) ListFiles(ctx context.Context, scope codegate.Scope) ([]string, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	root := scope.Path
	if root == "" {
		root = scope.Root
	}
	if root == "" {
		root = "."
	}
	var files []string
	err := fs.WalkDir(s.fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".agents", "vendor":
				return fs.SkipDir
			default:
				return nil
			}
		}
		files = append(files, p)
		return nil
	})
	return files, err
}

func (s dirSource) ReadFile(ctx context.Context, path string) ([]byte, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return fs.ReadFile(s.fsys, path)
}

func countGoFiles(fsys fs.FS, includeTests bool) (int, error) {
	n := 0
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".agents", "vendor":
				return fs.SkipDir
			default:
				return nil
			}
		}
		if !hasSuffix(p, ".go") || (!includeTests && hasSuffix(p, "_test.go")) {
			return nil
		}
		n++
		return nil
	})
	return n, err
}

func printTopMetrics(units []codegate.UnitMetrics, limit int) {
	units = append([]codegate.UnitMetrics(nil), units...)
	sort.Slice(units, func(i, j int) bool {
		if units[i].PressureScore == units[j].PressureScore {
			return units[i].UnitID < units[j].UnitID
		}
		return units[i].PressureScore > units[j].PressureScore
	})
	if len(units) > limit {
		units = units[:limit]
	}
	fmt.Println("top units by pressure:")
	if len(units) == 0 {
		fmt.Println("  none")
		return
	}
	for _, unit := range units {
		fmt.Printf("  %s score=%.1f files=%d loc=%d fanin=%d fanout=%d calls-in=%d calls-out=%d public=%d\n",
			unit.UnitID,
			unit.PressureScore,
			unit.FileCount,
			unit.LOC,
			unit.DirectFanIn,
			unit.DirectFanOut,
			unit.CallFanIn,
			unit.CallFanOut,
			unit.PublicSymbolCount,
		)
	}
}

func printValidationDiagnostics(diagnostics []codegate.Diagnostic, limit int) {
	if len(diagnostics) == 0 {
		return
	}
	if limit < 0 || limit > len(diagnostics) {
		limit = len(diagnostics)
	}
	for _, diagnostic := range diagnostics[:limit] {
		loc := diagnostic.Location
		fmt.Printf("  validation %s:%d:%d %s\n", loc.URI, loc.Range.Start.Line, loc.Range.Start.Column, diagnostic.Message)
	}
	if limit < len(diagnostics) {
		fmt.Printf("  validation ... %d more\n", len(diagnostics)-limit)
	}
}

func printProposals(proposals []codegate.Proposal, limit int) {
	if limit < 0 || limit > len(proposals) {
		limit = len(proposals)
	}
	fmt.Printf("refactor suggestions: %d", len(proposals))
	if limit < len(proposals) {
		fmt.Printf(" (showing %d)", limit)
	}
	fmt.Println()
	for _, proposal := range proposals[:limit] {
		fmt.Printf("  %s %s risk=%s confidence=%s ops=%d\n", proposal.Kind, proposal.Title, proposal.Risk, proposal.Confidence, len(proposal.Operations))
		if proposal.Summary != "" {
			fmt.Printf("    %s\n", proposal.Summary)
		}
	}
	if len(proposals) == 0 {
		fmt.Println("  none")
	}
}

func occurrenceCounts(occurrences []codegate.Occurrence) map[codegate.OccurrenceKind]int {
	counts := map[codegate.OccurrenceKind]int{}
	for _, occ := range occurrences {
		counts[occ.Kind]++
	}
	return counts
}

func formatOccurrenceCounts(counts map[codegate.OccurrenceKind]int) string {
	order := []codegate.OccurrenceKind{
		codegate.OccurrenceDeclaration,
		codegate.OccurrenceRead,
		codegate.OccurrenceWrite,
		codegate.OccurrenceCall,
		codegate.OccurrenceImport,
		codegate.OccurrenceDoc,
		codegate.OccurrenceReference,
	}
	parts := make([]string, 0, len(order))
	for _, kind := range order {
		if counts[kind] == 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%d", kind, counts[kind]))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, " ")
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
