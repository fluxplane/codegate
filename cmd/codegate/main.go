package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/codewandler/editor"
	"github.com/spf13/cobra"
)

type cliConfig struct {
	root         string
	language     string
	includeTests bool
	format       string
}

type app struct {
	cfg cliConfig
	out io.Writer
	err io.Writer
}

type suggestionSummary struct {
	ID         string                 `json:"id"`
	Kind       editor.RefactorKind    `json:"kind"`
	Title      string                 `json:"title"`
	Summary    string                 `json:"summary,omitempty"`
	Confidence editor.Confidence      `json:"confidence"`
	Risk       editor.RiskLevel       `json:"risk"`
	Operations int                    `json:"operations"`
	Metrics    map[string]float64     `json:"metrics,omitempty"`
	Evidence   []editor.Evidence      `json:"evidence,omitempty"`
	Raw        map[string]interface{} `json:"raw,omitempty"`
}

type lookupQuery = editor.LookupQuery

type cycleResult struct {
	Assessment editor.AssessmentReport      `json:"assessment"`
	Selected   *editor.AssessmentSuggestion `json:"selected,omitempty"`
	Applied    bool                         `json:"applied"`
	Validation *editor.ValidationSummary    `json:"validation,omitempty"`
	Diff       string                       `json:"diff,omitempty"`
	Message    string                       `json:"message,omitempty"`
}

func main() {
	a := &app{out: os.Stdout, err: os.Stderr}
	if err := a.rootCommand().Execute(); err != nil {
		fmt.Fprintf(a.err, "codegate: %v\n", err)
		os.Exit(1)
	}
}

func (a *app) rootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "codegate",
		Short: "Agent-oriented code analysis and improvement loop",
		Long: strings.TrimSpace(`codegate is a CLI skeleton for agent workflows.

It exposes the intended public cycle:

  lookup -> assess -> suggest -> apply -> validate -> reassess

The current implementation uses the public engine facade so the CLI can serve
as an agent skill and API proof at the same time.`),
	}
	cmd.PersistentFlags().StringVar(&a.cfg.root, "root", ".", "workspace root")
	cmd.PersistentFlags().StringVar(&a.cfg.language, "language", "go", "language backend")
	cmd.PersistentFlags().BoolVar(&a.cfg.includeTests, "tests", false, "include test files")
	cmd.PersistentFlags().StringVar(&a.cfg.format, "format", "json", "output format: json")
	cmd.AddCommand(
		a.capabilitiesCommand(),
		a.lookupCommand(),
		a.assessCommand(),
		a.suggestCommand(),
		a.validateCommand(),
		a.cycleCommand(),
	)
	return cmd
}

func (a *app) capabilitiesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "capabilities",
		Short: "List language backend capabilities",
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, _, err := a.engine(cmd.Context())
			if err != nil {
				return err
			}
			return a.print(eng.Capabilities())
		},
	}
	return cmd
}

func (a *app) lookupCommand() *cobra.Command {
	var q lookupQuery
	var kind string
	cmd := &cobra.Command{
		Use:   "lookup",
		Short: "Resolve a symbol, source position, or structural target",
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, scope, err := a.engine(cmd.Context())
			if err != nil {
				return err
			}
			q.Scope = scope
			q.Language = scope.Language
			q.IncludeTests = &a.cfg.includeTests
			out, err := eng.Lookup(cmd.Context(), q)
			if err != nil {
				return err
			}
			return a.print(out)
		},
	}
	cmd.Flags().StringVar(&q.Path, "path", "", "workspace-relative path")
	cmd.Flags().Int("offset", -1, "byte offset")
	cmd.Flags().IntVar(&q.Line, "line", 0, "1-indexed line")
	cmd.Flags().IntVar(&q.Column, "column", 0, "1-indexed byte column")
	cmd.Flags().StringVar(&q.Name, "name", "", "symbol or structural name")
	cmd.Flags().StringVar(&q.QualifiedName, "qualified-name", "", "qualified symbol name")
	cmd.Flags().StringVar(&kind, "kind", "", "symbol kind")
	cmd.Flags().BoolVar(&q.IncludeRefs, "refs", false, "include references for symbol lookup")
	cmd.Flags().BoolVar(&q.IncludeCallers, "callers", false, "include callers/callees for symbol lookup")
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		offset, err := cmd.Flags().GetInt("offset")
		if err != nil {
			return err
		}
		if offset >= 0 {
			q.Offset = &offset
		}
		q.Kind = editor.SymbolKind(kind)
		if q.Path == "" && q.Name == "" && q.QualifiedName == "" {
			return errors.New("lookup requires --path with a position or --name/--qualified-name")
		}
		return nil
	}
	return cmd
}

func (a *app) assessCommand() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "assess",
		Short: "Create an agent-readable quality assessment",
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := a.assess(cmd.Context(), limit)
			if err != nil {
				return err
			}
			return a.print(report)
		},
	}
	cmd.Flags().IntVar(&limit, "suggestions", 10, "maximum suggestions to include")
	return cmd
}

func (a *app) suggestCommand() *cobra.Command {
	var executableOnly bool
	var limit int
	cmd := &cobra.Command{
		Use:   "suggest",
		Short: "List improvement suggestions",
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, scope, err := a.engine(cmd.Context())
			if err != nil {
				return err
			}
			proposals, err := eng.Suggest(cmd.Context(), editor.SuggestOptions{Scope: scope})
			if err != nil {
				return err
			}
			out := summarizeSuggestions(proposals, executableOnly, limit)
			return a.print(out)
		},
	}
	cmd.Flags().BoolVar(&executableOnly, "executable", false, "only include suggestions with operations")
	cmd.Flags().IntVar(&limit, "limit", 20, "maximum suggestions")
	return cmd
}

func (a *app) validateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Run explicit validation checks",
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, scope, err := a.engine(cmd.Context())
			if err != nil {
				return err
			}
			result, err := eng.Validate(cmd.Context(), editor.ValidationOptions{
				Scope: scope,
				Kinds: []editor.ValidationKind{editor.ValidationParse, editor.ValidationTypecheck},
			})
			if err != nil {
				return err
			}
			return a.print(result)
		},
	}
	return cmd
}

func (a *app) cycleCommand() *cobra.Command {
	var applyFirst bool
	cmd := &cobra.Command{
		Use:   "cycle",
		Short: "Run assess -> suggest -> optionally apply -> validate",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			assessment, err := a.assess(ctx, 20)
			if err != nil {
				return err
			}
			result := cycleResult{Assessment: assessment}
			var selected *editor.AssessmentSuggestion
			for i := range assessment.Suggestions {
				if assessment.Suggestions[i].Operations > 0 {
					next := assessment.Suggestions[i]
					selected = &next
					break
				}
			}
			result.Selected = selected
			if selected == nil {
				result.Message = "no executable suggestion available"
				return a.print(result)
			}
			if !applyFirst {
				result.Message = "dry run: pass --apply-first to apply the first executable suggestion to an in-memory changeset"
				return a.print(result)
			}
			eng, scope, err := a.engine(ctx)
			if err != nil {
				return err
			}
			proposals, err := eng.Suggest(ctx, editor.SuggestOptions{Scope: scope})
			if err != nil {
				return err
			}
			var proposal editor.Proposal
			for _, candidate := range proposals {
				if candidate.ID == selected.ID {
					proposal = candidate
					break
				}
			}
			if len(proposal.Operations) == 0 {
				return fmt.Errorf("selected suggestion %s has no operations", selected.ID)
			}
			changes := eng.NewChangeSet()
			if err := changes.Apply(ctx, proposal.Operations...); err != nil {
				return err
			}
			validation, err := changes.Validate(ctx, editor.ValidationOptions{
				Scope: scope,
				Kinds: []editor.ValidationKind{editor.ValidationParse, editor.ValidationTypecheck},
			})
			if err != nil {
				return err
			}
			diff, err := changes.Diff(ctx)
			if err != nil {
				return err
			}
			result.Applied = true
			result.Validation = &editor.ValidationSummary{
				Passed:         validation.Passed,
				ResolutionMode: validation.ResolutionMode,
				Diagnostics:    len(validation.Diagnostics),
				Files:          len(validation.AffectedPaths),
				Complete:       validation.Complete,
			}
			result.Diff = diff
			return a.print(result)
		},
	}
	cmd.Flags().BoolVar(&applyFirst, "apply-first", false, "apply the first executable suggestion to an in-memory changeset and print the diff")
	return cmd
}

func (a *app) engine(ctx context.Context) (editor.Engine, editor.Scope, error) {
	if a.cfg.language != string(editor.Go) {
		return nil, editor.Scope{}, fmt.Errorf("language %q is not wired yet; current skeleton supports go", a.cfg.language)
	}
	eng, err := editor.NewEngine().Roots(a.cfg.root).WithSource(dirSource{fsys: os.DirFS(a.cfg.root)}).Build(ctx)
	if err != nil {
		return nil, editor.Scope{}, err
	}
	return eng, editor.Scope{Language: editor.Go, IncludeTests: a.cfg.includeTests}, nil
}

func (a *app) assess(ctx context.Context, limit int) (editor.AssessmentReport, error) {
	eng, scope, err := a.engine(ctx)
	if err != nil {
		return editor.AssessmentReport{}, err
	}
	return eng.Assess(ctx, editor.AssessmentOptions{Scope: scope, SuggestionLimit: limit})
}

func (a *app) print(v interface{}) error {
	if a.cfg.format != "json" {
		return fmt.Errorf("unsupported format %q", a.cfg.format)
	}
	enc := json.NewEncoder(a.out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

type dirSource struct {
	fsys fs.FS
}

func (s dirSource) ListFiles(ctx context.Context, scope editor.Scope) ([]string, error) {
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

func summarizeSuggestions(proposals []editor.Proposal, executableOnly bool, limit int) []suggestionSummary {
	out := make([]suggestionSummary, 0, len(proposals))
	for _, proposal := range proposals {
		if executableOnly && !editor.HasOperations(proposal) {
			continue
		}
		out = append(out, suggestionSummary{
			ID:         proposal.ID,
			Kind:       proposal.Kind,
			Title:      proposal.Title,
			Summary:    proposal.Summary,
			Confidence: proposal.Confidence,
			Risk:       proposal.Risk,
			Operations: len(proposal.Operations),
			Metrics:    proposal.Metrics,
			Evidence:   proposal.Evidence,
		})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}
