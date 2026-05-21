package editor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/codewandler/editor/internal/core"
)

type ChangeSet struct {
	editor  *Editor
	overlay map[string][]byte
	changed map[string]bool
	closed  bool
}

func (c *ChangeSet) Apply(ctx context.Context, ops ...Operation) error {
	if c.closed {
		return errors.New("editor: changeset is closed")
	}
	for _, op := range ops {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		backend, err := c.editor.backendForOperation(op)
		if err != nil {
			return err
		}
		edits, err := backend.CompileEdit(ctx, c.editor.snapshot(c.overlay), op)
		if err != nil {
			return err
		}
		if err := c.applyFileEdits(ctx, edits); err != nil {
			return err
		}
	}
	return nil
}

func (c *ChangeSet) Read(ctx context.Context, sel SymbolSelector) (SourceFragment, error) {
	if c.closed {
		return SourceFragment{}, errors.New("editor: changeset is closed")
	}
	return c.editor.readSymbol(ctx, sel, c.overlay)
}

func (c *ChangeSet) Diff(ctx context.Context) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	files, err := c.Files(ctx)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, file := range files {
		if bytes.Equal(file.Before, file.After) {
			continue
		}
		writeUnifiedDiff(&b, file.Path, file.Before, file.After)
	}
	return b.String(), nil
}

func (c *ChangeSet) Files(ctx context.Context) ([]ChangedFile, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	paths := make([]string, 0, len(c.changed))
	for p := range c.changed {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	files := make([]ChangedFile, 0, len(paths))
	for _, p := range paths {
		before, err := c.editor.readFileWithOverlay(ctx, p, nil)
		if err != nil {
			before = nil
		}
		after := append([]byte(nil), c.overlay[p]...)
		files = append(files, ChangedFile{Path: p, Before: before, After: after})
	}
	return files, nil
}

func (c *ChangeSet) Commit(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if c.closed {
		return errors.New("editor: changeset is closed")
	}
	c.editor.mu.Lock()
	defer c.editor.mu.Unlock()
	for p := range c.changed {
		c.editor.overlay[p] = append([]byte(nil), c.overlay[p]...)
	}
	c.closed = true
	return nil
}

func (c *ChangeSet) Discard() error {
	c.closed = true
	c.overlay = nil
	c.changed = nil
	return nil
}

func (c *ChangeSet) applyFileEdits(ctx context.Context, fileEdits []FileEdit) error {
	for _, fe := range fileEdits {
		p := core.CleanPath(fe.Path)
		src, err := c.editor.readFileWithOverlay(ctx, p, c.overlay)
		if err != nil {
			return err
		}
		next, err := applyTextEdits(src, fe.Edits)
		if err != nil {
			return err
		}
		if backend, ok := c.editor.backendForPath(p); ok {
			formatted, err := backend.Format(ctx, p, next)
			if err != nil {
				return fmt.Errorf("editor: format %s: %w", p, err)
			}
			next = formatted
		}
		c.overlay[p] = next
		c.changed[p] = true
	}
	return nil
}

func applyTextEdits(src []byte, edits []TextEdit) ([]byte, error) {
	if len(edits) == 0 {
		return src, nil
	}
	edits = append([]TextEdit(nil), edits...)
	// Keep same-start edits in backend-provided order. This makes multiple
	// zero-length inserts at one offset deterministic: their replacements are
	// emitted in the same order they appeared in the input edit slice.
	sort.SliceStable(edits, func(i, j int) bool {
		return edits[i].Range.Start.Offset < edits[j].Range.Start.Offset
	})
	prevEnd := -1
	for _, edit := range edits {
		start, end := edit.Range.Start.Offset, edit.Range.End.Offset
		if start < 0 || end < start || end > len(src) {
			return nil, fmt.Errorf("editor: invalid text edit range %d..%d for %d byte file", start, end, len(src))
		}
		if start < prevEnd {
			return nil, errors.New("editor: overlapping text edits")
		}
		prevEnd = end
	}
	var out bytes.Buffer
	cursor := 0
	for _, edit := range edits {
		start, end := edit.Range.Start.Offset, edit.Range.End.Offset
		out.Write(src[cursor:start])
		out.WriteString(edit.Replacement)
		cursor = end
	}
	out.Write(src[cursor:])
	return out.Bytes(), nil
}

func writeUnifiedDiff(b *strings.Builder, p string, before, after []byte) {
	b.WriteString("--- a/")
	b.WriteString(p)
	b.WriteString("\n+++ b/")
	b.WriteString(p)
	b.WriteString("\n")
	beforeLines := splitLines(before)
	afterLines := splitLines(after)
	b.WriteString(fmt.Sprintf("@@ -1,%d +1,%d @@\n", len(beforeLines), len(afterLines)))
	for _, line := range beforeLines {
		b.WriteByte('-')
		b.WriteString(line)
		if !strings.HasSuffix(line, "\n") {
			b.WriteByte('\n')
		}
	}
	for _, line := range afterLines {
		b.WriteByte('+')
		b.WriteString(line)
		if !strings.HasSuffix(line, "\n") {
			b.WriteByte('\n')
		}
	}
}

func splitLines(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	parts := strings.SplitAfter(string(b), "\n")
	if parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}
