package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/fluxplane/codegate"
	"github.com/spf13/cobra"
)

type operationRunResult struct {
	Operations   []codegate.OperationKind   `json:"operations"`
	Changed      []operationChangedFile     `json:"changed"`
	Validation   codegate.ValidationSummary `json:"validation"`
	Diagnostics  []codegate.Diagnostic      `json:"diagnostics,omitempty"`
	Diff         string                     `json:"diff,omitempty"`
	DryRun       bool                       `json:"dry_run"`
	Written      bool                       `json:"written"`
	WrittenFiles []string                   `json:"written_files,omitempty"`
	PatchPath    string                     `json:"patch_path,omitempty"`
}

type operationChangedFile struct {
	Path  string `json:"path"`
	Bytes int    `json:"bytes"`
}

type operationRunOptions struct {
	operation     string
	operationFile string
	kind          string
	from          string
	to            string
	write         bool
	patch         string
	noDiff        bool
}

func (a *app) operationCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "op",
		Short: "Run explicit structured edit operations",
	}
	cmd.AddCommand(a.operationRunCommand())
	return cmd
}

func (a *app) operationRunCommand() *cobra.Command {
	var opts operationRunOptions
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Apply explicit operations to an in-memory changeset",
		RunE: func(cmd *cobra.Command, args []string) error {
			ops, err := readOperations(cmd.Context(), opts)
			if err != nil {
				return err
			}
			if len(ops) == 0 {
				return errors.New("op run requires at least one operation")
			}
			eng, scope, err := a.engine(cmd.Context())
			if err != nil {
				return err
			}
			changes := eng.NewChangeSet()
			if err := changes.Apply(cmd.Context(), ops...); err != nil {
				return err
			}
			validation, err := changes.Validate(cmd.Context(), codegate.ValidationOptions{
				Scope: scope,
				Kinds: validationKinds(scope.Language),
			})
			if err != nil {
				return err
			}
			files, err := changes.Files(cmd.Context())
			if err != nil {
				return err
			}
			var diff string
			if !opts.noDiff || opts.patch != "" {
				diff, err = changes.Diff(cmd.Context())
				if err != nil {
					return err
				}
			}
			result := operationRunResult{
				Operations: operationKinds(ops),
				Changed:    summarizeChangedFiles(files),
				Validation: codegate.ValidationSummary{
					Passed:         validation.Passed,
					ResolutionMode: validation.ResolutionMode,
					Diagnostics:    len(validation.Diagnostics),
					Files:          len(validation.AffectedPaths),
					Complete:       validation.Complete,
				},
				Diagnostics: validation.Diagnostics,
				DryRun:      !opts.write,
			}
			if !opts.noDiff {
				result.Diff = diff
			}
			if opts.patch != "" {
				if err := writePatchFile(opts.patch, []byte(diff)); err != nil {
					return err
				}
				result.PatchPath = opts.patch
			}
			if opts.write {
				if !validation.Passed {
					return errors.New("op run refused --write because validation failed")
				}
				written, err := writeChangedFiles(cmd.Context(), a.cfg.root, files)
				if err != nil {
					if len(written) > 0 {
						result.WrittenFiles = written
						_ = a.print(result)
					}
					return err
				}
				result.Written = true
				result.WrittenFiles = written
			}
			return a.print(result)
		},
	}
	cmd.Flags().StringVar(&opts.operation, "operation", "", "operation JSON object or array")
	cmd.Flags().StringVar(&opts.operationFile, "operation-file", "", "operation JSON file, or - for stdin")
	cmd.Flags().StringVar(&opts.kind, "kind", "", "operation kind for convenience flags")
	cmd.Flags().StringVar(&opts.from, "from", "", "old value for convenience operations")
	cmd.Flags().StringVar(&opts.to, "to", "", "new value for convenience operations")
	cmd.Flags().BoolVar(&opts.write, "write", false, "write changed files to the workspace after validation")
	cmd.Flags().StringVar(&opts.patch, "patch", "", "write unified diff to this patch file")
	cmd.Flags().BoolVar(&opts.noDiff, "no-diff", false, "omit unified diff from JSON output")
	return cmd
}

func readOperations(ctx context.Context, opts operationRunOptions) ([]codegate.Operation, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	inputs := 0
	if opts.operation != "" {
		inputs++
	}
	if opts.operationFile != "" {
		inputs++
	}
	if opts.kind != "" {
		inputs++
	}
	if inputs != 1 {
		return nil, errors.New("op run requires exactly one of --operation, --operation-file, or --kind")
	}
	if opts.kind != "" {
		return operationFromConvenienceFlags(opts)
	}
	var data []byte
	var err error
	if opts.operation != "" {
		data = []byte(opts.operation)
	} else if opts.operationFile == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(opts.operationFile)
	}
	if err != nil {
		return nil, err
	}
	return decodeOperations(data)
}

func operationFromConvenienceFlags(opts operationRunOptions) ([]codegate.Operation, error) {
	switch codegate.OperationKind(opts.kind) {
	case codegate.OpRenameGoModulePath:
		if opts.from == "" || opts.to == "" {
			return nil, errors.New("go_module_path_rename requires --from and --to")
		}
		return []codegate.Operation{codegate.RenameGoModulePath{OldPath: opts.from, NewPath: opts.to}}, nil
	default:
		return nil, fmt.Errorf("unsupported convenience operation kind %q", opts.kind)
	}
}

func decodeOperations(data []byte) ([]codegate.Operation, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, errors.New("operation JSON is empty")
	}
	if data[0] == '[' {
		var raws []json.RawMessage
		if err := json.Unmarshal(data, &raws); err != nil {
			return nil, fmt.Errorf("parse operation array: %w", err)
		}
		ops := make([]codegate.Operation, 0, len(raws))
		for _, raw := range raws {
			op, err := decodeOperation(raw)
			if err != nil {
				return nil, err
			}
			ops = append(ops, op)
		}
		return ops, nil
	}
	op, err := decodeOperation(data)
	if err != nil {
		return nil, err
	}
	return []codegate.Operation{op}, nil
}

func decodeOperation(data []byte) (codegate.Operation, error) {
	var header struct {
		Kind codegate.OperationKind `json:"kind"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return nil, fmt.Errorf("parse operation: %w", err)
	}
	if header.Kind == "" {
		return nil, errors.New("operation JSON requires kind")
	}
	switch header.Kind {
	case codegate.OpRenameSymbol:
		return decodeOperationAs[codegate.RenameSymbol](data)
	case codegate.OpReplaceSymbol:
		return decodeOperationAs[codegate.ReplaceSymbol](data)
	case codegate.OpDeleteSymbol:
		return decodeOperationAs[codegate.DeleteSymbol](data)
	case codegate.OpAppendSymbol:
		return decodeOperationAs[codegate.AppendSymbol](data)
	case codegate.OpReplaceFunction:
		return decodeOperationAs[codegate.ReplaceFunction](data)
	case codegate.OpAppendFunction:
		return decodeOperationAs[codegate.AppendFunction](data)
	case codegate.OpDeleteFunction:
		return decodeOperationAs[codegate.DeleteFunction](data)
	case codegate.OpReplaceMethod:
		return decodeOperationAs[codegate.ReplaceMethod](data)
	case codegate.OpDeleteMethod:
		return decodeOperationAs[codegate.DeleteMethod](data)
	case codegate.OpReplaceComment:
		return decodeOperationAs[codegate.ReplaceComment](data)
	case codegate.OpEnsureStructTag:
		return decodeOperationAs[codegate.EnsureGoStructTag](data)
	case codegate.OpRemoveStructTag:
		return decodeOperationAs[codegate.RemoveGoStructTag](data)
	case codegate.OpEnsureGoImport:
		return decodeOperationAs[codegate.EnsureGoImport](data)
	case codegate.OpRemoveGoImport:
		return decodeOperationAs[codegate.RemoveGoImport](data)
	case codegate.OpRenameGoImport:
		return decodeOperationAs[codegate.RenameGoImport](data)
	case codegate.OpRenameGoModulePath:
		return decodeOperationAs[codegate.RenameGoModulePath](data)
	case codegate.OpMoveSymbol:
		return decodeOperationAs[codegate.MoveSymbol](data)
	case codegate.OpAddGoParameter:
		return decodeOperationAs[codegate.AddGoParameter](data)
	case codegate.OpRemoveGoParam:
		return decodeOperationAs[codegate.RemoveGoParameter](data)
	case codegate.OpRenameGoParam:
		return decodeOperationAs[codegate.RenameGoParameter](data)
	case codegate.OpAddGoField:
		return decodeOperationAs[codegate.AddGoStructField](data)
	case codegate.OpRemoveGoField:
		return decodeOperationAs[codegate.RemoveGoStructField](data)
	case codegate.OpRenameGoField:
		return decodeOperationAs[codegate.RenameGoStructField](data)
	case codegate.OpChangeGoParam:
		return decodeOperationAs[codegate.ChangeGoParameterType](data)
	case codegate.OpChangeGoResult:
		return decodeOperationAs[codegate.ChangeGoResultType](data)
	case codegate.OpRenameGoRecv:
		return decodeOperationAs[codegate.RenameGoReceiver](data)
	case codegate.OpAddGoIfaceMeth:
		return decodeOperationAs[codegate.AddGoInterfaceMethod](data)
	case codegate.OpRemoveGoIface:
		return decodeOperationAs[codegate.RemoveGoInterfaceMethod](data)
	case codegate.OpExtractGoFunc:
		return decodeOperationAs[codegate.ExtractGoFunction](data)
	case codegate.OpExtractGoMethod:
		return decodeOperationAs[codegate.ExtractGoMethod](data)
	case codegate.OpMarkdownEnsureH1:
		return decodeOperationAs[codegate.EnsureMarkdownH1](data)
	case codegate.OpMarkdownSetHeadingLevel:
		return decodeOperationAs[codegate.SetMarkdownHeadingLevel](data)
	case codegate.OpMarkdownInsertSectionBody:
		return decodeOperationAs[codegate.InsertMarkdownSectionBody](data)
	case codegate.OpMarkdownRenameHeading:
		return decodeOperationAs[codegate.RenameMarkdownHeading](data)
	default:
		return nil, fmt.Errorf("unsupported operation kind %q", header.Kind)
	}
}

func decodeOperationAs[T codegate.Operation](data []byte) (codegate.Operation, error) {
	var op T
	if err := json.Unmarshal(data, &op); err != nil {
		return nil, err
	}
	return op, nil
}

func operationKinds(ops []codegate.Operation) []codegate.OperationKind {
	out := make([]codegate.OperationKind, 0, len(ops))
	for _, op := range ops {
		out = append(out, op.Kind())
	}
	return out
}

func summarizeChangedFiles(files []codegate.ChangedFile) []operationChangedFile {
	out := make([]operationChangedFile, 0, len(files))
	for _, file := range files {
		out = append(out, operationChangedFile{Path: file.Path, Bytes: len(file.After)})
	}
	return out
}

func writePatchFile(path string, data []byte) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func writeChangedFiles(ctx context.Context, root string, files []codegate.ChangedFile) ([]string, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	written := make([]string, 0, len(files))
	for _, file := range files {
		if ctx.Err() != nil {
			return written, ctx.Err()
		}
		target, err := workspacePath(rootAbs, file.Path)
		if err != nil {
			return written, err
		}
		if err := atomicWriteFile(target, file.After); err != nil {
			return written, err
		}
		written = append(written, file.Path)
	}
	return written, nil
}

func workspacePath(rootAbs, rel string) (string, error) {
	cleanRel := filepath.Clean(filepath.FromSlash(rel))
	if cleanRel == "." || filepath.IsAbs(cleanRel) || strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) || cleanRel == ".." {
		return "", fmt.Errorf("refusing to write path outside workspace: %s", rel)
	}
	target := filepath.Join(rootAbs, cleanRel)
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	if targetAbs != rootAbs && !strings.HasPrefix(targetAbs, rootAbs+string(filepath.Separator)) {
		return "", fmt.Errorf("refusing to write path outside workspace: %s", rel)
	}
	return targetAbs, nil
}

func atomicWriteFile(path string, data []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	mode := info.Mode().Perm()
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".codegate-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
