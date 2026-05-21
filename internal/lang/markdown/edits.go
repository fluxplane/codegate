package markdown

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/codewandler/codegate/internal/core"
)

func compileMarkdownEdit(ctx context.Context, snapshot Snapshot, op Operation) ([]FileEdit, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	switch x := op.(type) {
	case EnsureMarkdownH1:
		return compileEnsureMarkdownH1(ctx, snapshot, x)
	case SetMarkdownHeadingLevel:
		return compileSetMarkdownHeadingLevel(ctx, snapshot, x)
	case InsertMarkdownSectionBody:
		return compileInsertMarkdownSectionBody(ctx, snapshot, x)
	case RenameMarkdownHeading:
		return compileRenameMarkdownHeading(ctx, snapshot, x)
	default:
		return nil, fmt.Errorf("markdown: unsupported edit operation %q", op.Kind())
	}
}

func compileEnsureMarkdownH1(ctx context.Context, snapshot Snapshot, op EnsureMarkdownH1) ([]FileEdit, error) {
	if op.Path == "" {
		return nil, errors.New("markdown: h1 ensure requires path")
	}
	src, err := snapshot.ReadFile(ctx, op.Path)
	if err != nil {
		return nil, err
	}
	file := parseMarkdownFile(op.Path, src)
	for _, heading := range file.headings {
		if heading.level == 1 {
			return nil, errors.New("markdown: document already has an H1")
		}
	}
	title := strings.TrimSpace(op.Title)
	if title == "" {
		title = markdownTitleFromPath(op.Path)
	}
	replacement := "# " + title + "\n\n"
	return []FileEdit{{Path: op.Path, Edits: []TextEdit{{
		Path:        op.Path,
		Range:       Range{Start: Position{Line: 1, Column: 1, Offset: 0}, End: Position{Line: 1, Column: 1, Offset: 0}},
		Replacement: replacement,
	}}}}, nil
}

func compileSetMarkdownHeadingLevel(ctx context.Context, snapshot Snapshot, op SetMarkdownHeadingLevel) ([]FileEdit, error) {
	if op.Path == "" {
		return nil, errors.New("markdown: heading level edit requires path")
	}
	if op.Level < 1 || op.Level > 6 {
		return nil, fmt.Errorf("markdown: invalid heading level %d", op.Level)
	}
	src, err := snapshot.ReadFile(ctx, op.Path)
	if err != nil {
		return nil, err
	}
	heading, ok := markdownHeadingAtOffset(parseMarkdownFile(op.Path, src), op.Offset)
	if !ok {
		return nil, errors.New("markdown: heading not found")
	}
	line := headingLine(src, heading)
	prefixEnd := line.Start.Offset
	for prefixEnd < line.End.Offset && src[prefixEnd] == '#' {
		prefixEnd++
	}
	if prefixEnd == line.Start.Offset || prefixEnd >= len(src) || src[prefixEnd] != ' ' {
		return nil, errors.New("markdown: unsupported heading marker")
	}
	return []FileEdit{{Path: op.Path, Edits: []TextEdit{{
		Path:        op.Path,
		Range:       Range{Start: line.Start, End: Position{Line: line.Start.Line, Column: line.Start.Column + prefixEnd - line.Start.Offset, Offset: prefixEnd}},
		Replacement: strings.Repeat("#", op.Level),
	}}}}, nil
}

func compileInsertMarkdownSectionBody(ctx context.Context, snapshot Snapshot, op InsertMarkdownSectionBody) ([]FileEdit, error) {
	if op.Path == "" {
		return nil, errors.New("markdown: section body insert requires path")
	}
	text := strings.TrimSpace(op.Text)
	if text == "" {
		text = "TBD."
	}
	src, err := snapshot.ReadFile(ctx, op.Path)
	if err != nil {
		return nil, err
	}
	heading, ok := markdownHeadingAtOffset(parseMarkdownFile(op.Path, src), op.Offset)
	if !ok {
		return nil, errors.New("markdown: heading not found")
	}
	insert := heading.location.Range.End
	replacement := "\n" + text + "\n"
	if insert.Offset < len(src) && src[insert.Offset] == '\n' {
		replacement = text + "\n"
	}
	return []FileEdit{{Path: op.Path, Edits: []TextEdit{{
		Path:        op.Path,
		Range:       Range{Start: insert, End: insert},
		Replacement: replacement,
	}}}}, nil
}

func compileRenameMarkdownHeading(ctx context.Context, snapshot Snapshot, op RenameMarkdownHeading) ([]FileEdit, error) {
	if op.Path == "" {
		return nil, errors.New("markdown: heading rename requires path")
	}
	if strings.TrimSpace(op.NewText) == "" {
		return nil, errors.New("markdown: heading rename requires text")
	}
	src, err := snapshot.ReadFile(ctx, op.Path)
	if err != nil {
		return nil, err
	}
	heading, ok := markdownHeadingAtOffset(parseMarkdownFile(op.Path, src), op.Offset)
	if !ok {
		return nil, errors.New("markdown: heading not found")
	}
	textRange, ok := headingTextRange(src, heading)
	if !ok {
		return nil, errors.New("markdown: unsupported heading text")
	}
	return []FileEdit{{Path: op.Path, Edits: []TextEdit{{
		Path:        op.Path,
		Range:       textRange,
		Replacement: strings.TrimSpace(op.NewText),
	}}}}, nil
}

func markdownSuggestions(ctx context.Context, snapshot Snapshot, scope Scope) ([]Proposal, error) {
	idx, err := buildIndex(ctx, snapshot, scope)
	if err != nil {
		return nil, err
	}
	var proposals []Proposal
	for _, file := range idx.files {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		proposals = append(proposals, markdownFileSuggestions(file)...)
	}
	for i := range proposals {
		proposals[i].ID = fmt.Sprintf("prop_%03d", i+1)
	}
	return proposals, nil
}

func markdownFileSuggestions(file markdownFile) []Proposal {
	var out []Proposal
	h1Count := 0
	for _, heading := range file.headings {
		if heading.level == 1 {
			h1Count++
		}
	}
	if h1Count == 0 {
		title := markdownTitleFromPath(file.path)
		out = append(out, Proposal{
			Kind:       RefactorFixMarkdownStructure,
			Title:      "Add missing Markdown H1",
			Summary:    fmt.Sprintf("%s has no H1 title.", file.path),
			Confidence: core.HighConfidence,
			Risk:       core.RiskLow,
			Evidence:   []Evidence{{Kind: "markdown_missing_h1", Message: "Document has no H1 title.", Location: Location{URI: file.path}}},
			Operations: []Operation{EnsureMarkdownH1{Path: file.path, Title: title}},
		})
	}
	for i, heading := range file.headings {
		if i > 0 && heading.level > file.headings[i-1].level+1 {
			level := file.headings[i-1].level + 1
			out = append(out, Proposal{
				Kind:       RefactorFixMarkdownStructure,
				Title:      "Normalize Markdown heading level",
				Summary:    fmt.Sprintf("Change %q from H%d to H%d.", heading.name, heading.level, level),
				Confidence: core.HighConfidence,
				Risk:       core.RiskLow,
				Targets:    []Symbol{headingSymbol(file.path, heading)},
				Evidence:   []Evidence{{Kind: "markdown_heading_level_jump", Message: "Heading level skips its parent level.", Location: heading.location}},
				Operations: []Operation{SetMarkdownHeadingLevel{Path: file.path, Offset: heading.location.Range.Start.Offset, Level: level}},
			})
		}
		if sectionText(file.src, heading) == "" {
			out = append(out, Proposal{
				Kind:       RefactorFixMarkdownStructure,
				Title:      "Add Markdown section body",
				Summary:    fmt.Sprintf("Add placeholder body content under %q.", heading.name),
				Confidence: core.MediumConfidence,
				Risk:       core.RiskLow,
				Targets:    []Symbol{headingSymbol(file.path, heading)},
				Evidence:   []Evidence{{Kind: "markdown_empty_section", Message: "Heading has no body content.", Location: heading.location}},
				Operations: []Operation{InsertMarkdownSectionBody{Path: file.path, Offset: heading.location.Range.Start.Offset, Text: "TBD."}},
			})
		}
	}
	out = append(out, duplicateHeadingSuggestions(file)...)
	out = append(out, brokenLinkSuggestions(file)...)
	sort.SliceStable(out, func(i, j int) bool {
		if len(out[i].Evidence) == 0 || len(out[j].Evidence) == 0 {
			return out[i].Title < out[j].Title
		}
		return out[i].Evidence[0].Location.Range.Start.Offset < out[j].Evidence[0].Location.Range.Start.Offset
	})
	return out
}

func brokenLinkSuggestions(file markdownFile) []Proposal {
	anchors := map[string]bool{}
	for _, heading := range file.headings {
		anchors[strings.TrimPrefix(heading.qualified, file.path+"#")] = true
		anchors[heading.anchor] = true
	}
	var out []Proposal
	for _, link := range file.links {
		dest := strings.TrimSpace(link.destination)
		if dest == "" || anchors[dest] {
			continue
		}
		out = append(out, Proposal{
			Kind:       RefactorFixMarkdownStructure,
			Title:      "Review broken Markdown heading link",
			Summary:    fmt.Sprintf("Local heading link #%s does not resolve.", dest),
			Confidence: core.MediumConfidence,
			Risk:       core.RiskLow,
			Evidence:   []Evidence{{Kind: "markdown_broken_local_heading_link", Message: "Local heading link does not resolve.", Location: link.location}},
		})
	}
	return out
}

func duplicateHeadingSuggestions(file markdownFile) []Proposal {
	if len(file.headings) < 2 {
		return nil
	}
	linkTargets := map[string]bool{}
	for _, link := range file.links {
		linkTargets[strings.TrimSpace(link.destination)] = true
	}
	seen := map[string]int{}
	var out []Proposal
	for _, heading := range file.headings {
		seen[heading.anchor]++
		if seen[heading.anchor] <= 1 {
			continue
		}
		numberedAnchor := fmt.Sprintf("%s-%d", heading.anchor, seen[heading.anchor]-1)
		if linkTargets[heading.anchor] || linkTargets[numberedAnchor] {
			continue
		}
		newName := fmt.Sprintf("%s %d", heading.name, seen[heading.anchor])
		out = append(out, Proposal{
			Kind:       RefactorFixMarkdownStructure,
			Title:      "Disambiguate duplicate Markdown heading",
			Summary:    fmt.Sprintf("Rename duplicate heading %q to %q.", heading.name, newName),
			Confidence: core.MediumConfidence,
			Risk:       core.RiskLow,
			Targets:    []Symbol{headingSymbol(file.path, heading)},
			Evidence:   []Evidence{{Kind: "markdown_duplicate_heading_anchor", Message: "Heading anchor is duplicated.", Location: heading.location}},
			Operations: []Operation{RenameMarkdownHeading{Path: file.path, Offset: heading.location.Range.Start.Offset, NewText: newName}},
		})
	}
	return out
}

func markdownHeadingAtOffset(file markdownFile, offset int) (headingInfo, bool) {
	for _, heading := range file.headings {
		if heading.location.Range.Start.Offset == offset {
			return heading, true
		}
	}
	return headingInfo{}, false
}

func headingLine(src []byte, heading headingInfo) Range {
	start := heading.location.Range.Start.Offset
	end := heading.location.Range.End.Offset
	for end > start && src[end-1] == '\n' {
		end--
	}
	return Range{Start: heading.location.Range.Start, End: positionForOffset(src, end)}
}

func headingTextRange(src []byte, heading headingInfo) (Range, bool) {
	line := headingLine(src, heading)
	start := line.Start.Offset
	for start < line.End.Offset && src[start] == '#' {
		start++
	}
	if start >= line.End.Offset || src[start] != ' ' {
		return Range{}, false
	}
	start++
	for start < line.End.Offset && src[start] == ' ' {
		start++
	}
	end := line.End.Offset
	for end > start && src[end-1] == ' ' {
		end--
	}
	return Range{Start: positionForOffset(src, start), End: positionForOffset(src, end)}, true
}

func markdownTitleFromPath(p string) string {
	base := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
	base = strings.ReplaceAll(base, "-", " ")
	base = strings.ReplaceAll(base, "_", " ")
	base = strings.TrimSpace(base)
	if base == "" || base == "." {
		return "Document"
	}
	parts := strings.Fields(base)
	for i, part := range parts {
		if part == strings.ToUpper(part) {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}
