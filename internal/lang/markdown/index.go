package markdown

import (
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/codewandler/editor/internal/core"
	"github.com/yuin/goldmark"
	goldast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

type MarkdownBackend struct{}

func New() MarkdownBackend {
	return MarkdownBackend{}
}

func (b MarkdownBackend) Spec() BackendSpec {
	return BackendSpec{
		Language:       Markdown,
		Name:           "goldmark",
		FileExtensions: []string{".md", ".markdown"},
		Capabilities: []CapabilitySupport{
			{Capability: CapabilityLookup, Level: CapabilityBasic, Notes: "Indexes documents, headings, anchors, and enclosing sections."},
			{Capability: CapabilityStaticAnalysis, Level: CapabilityBasic, Notes: "Parses Markdown structure through goldmark."},
			{Capability: CapabilityQuality, Level: CapabilityBasic, Notes: "Reports structural quality findings for headings, sections, and local links."},
			{Capability: CapabilityEditing, Level: CapabilityNone, Notes: "Markdown editing operations are not implemented yet."},
			{Capability: CapabilityRefactoring, Level: CapabilityNone, Notes: "Markdown refactoring operations are not implemented yet."},
			{Capability: CapabilityValidation, Level: CapabilityBasic, Notes: "Validates files can be parsed and indexed as Markdown."},
			{Capability: CapabilityReporting, Level: CapabilityBasic, Notes: "Feeds Markdown structural findings into assessment reports."},
		},
		ResolutionMode: "structural",
	}
}

func (b MarkdownBackend) Index(ctx context.Context, snapshot Snapshot, scope Scope) (*Index, error) {
	idx, err := buildIndex(ctx, snapshot, scope)
	if err != nil {
		return nil, err
	}
	return exportIndex(idx), nil
}

func (b MarkdownBackend) CompileEdit(context.Context, Snapshot, Operation) ([]FileEdit, error) {
	return nil, errors.New("markdown: edit operations are not supported")
}

func (b MarkdownBackend) Format(_ context.Context, _ string, src []byte) ([]byte, error) {
	return src, nil
}

func (b MarkdownBackend) Suggest(context.Context, Snapshot, Scope) ([]core.Proposal, error) {
	return nil, nil
}

type index struct {
	documents   []Document
	packages    []PackageInfo
	symbols     []Symbol
	occurrences []Occurrence
	diagnostics []Diagnostic
	byID        map[SymbolID]Symbol
	byName      map[string][]Symbol
	unitFiles   map[string][]string
	fileUnits   map[string]string
	fileLOC     map[string]int
	files       []markdownFile
}

type markdownFile struct {
	path     string
	src      []byte
	headings []headingInfo
	links    []linkInfo
}

type headingInfo struct {
	level        int
	name         string
	anchor       string
	qualified    string
	location     Location
	selection    Range
	sectionRange Range
}

type linkInfo struct {
	destination string
	location    Location
}

func buildIndex(ctx context.Context, snapshot Snapshot, scope Scope) (*index, error) {
	if scope.Language != "" && scope.Language != Markdown {
		return emptyIndex(), nil
	}
	files, err := snapshot.ListFiles(ctx, scope)
	if err != nil {
		return nil, err
	}
	idx := emptyIndex()
	indexedFiles := 0
	for _, p := range files {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !isMarkdownPath(p) {
			continue
		}
		if scope.MaxFiles > 0 && indexedFiles >= scope.MaxFiles {
			break
		}
		src, err := snapshot.ReadFile(ctx, p)
		if err != nil {
			idx.diagnostics = append(idx.diagnostics, Diagnostic{Severity: "error", Message: err.Error()})
			continue
		}
		if scope.MaxBytes > 0 && int64(len(src)) > scope.MaxBytes {
			continue
		}
		mf := parseMarkdownFile(p, src)
		idx.files = append(idx.files, mf)
		idx.documents = append(idx.documents, Document{URI: p, Language: Markdown, UnitID: p})
		idx.packages = append(idx.packages, PackageInfo{ID: p, Name: filepath.Base(p), Dir: path.Dir(p), Files: []string{p}})
		idx.unitFiles[p] = []string{p}
		idx.fileUnits[p] = p
		idx.fileLOC[p] = countLines(src)
		doc := documentSymbol(p, src)
		addSymbol(idx, doc)
		for _, heading := range mf.headings {
			addSymbol(idx, headingSymbol(p, heading))
		}
		indexedFiles++
	}
	sortSymbols(idx.symbols)
	sortOccurrences(idx.occurrences)
	sort.Slice(idx.packages, func(i, j int) bool { return idx.packages[i].ID < idx.packages[j].ID })
	return idx, nil
}

func emptyIndex() *index {
	return &index{
		byID:      map[SymbolID]Symbol{},
		byName:    map[string][]Symbol{},
		unitFiles: map[string][]string{},
		fileUnits: map[string]string{},
		fileLOC:   map[string]int{},
	}
}

func exportIndex(idx *index) *Index {
	return &Index{
		Documents:   append([]Document(nil), idx.documents...),
		Packages:    append([]PackageInfo(nil), idx.packages...),
		Symbols:     append([]Symbol(nil), idx.symbols...),
		Occurrences: append([]Occurrence(nil), idx.occurrences...),
		Diagnostics: append([]Diagnostic(nil), idx.diagnostics...),
		ByID:        idx.byID,
		ByName:      idx.byName,
		UnitFiles:   idx.unitFiles,
		FileUnits:   idx.fileUnits,
		FileLOC:     idx.fileLOC,
	}
}

func parseMarkdownFile(p string, src []byte) markdownFile {
	root := goldmark.DefaultParser().Parse(text.NewReader(src))
	var headings []headingInfo
	var links []linkInfo
	anchorCounts := map[string]int{}
	_ = goldast.Walk(root, func(n goldast.Node, entering bool) (goldast.WalkStatus, error) {
		if !entering {
			return goldast.WalkContinue, nil
		}
		switch node := n.(type) {
		case *goldast.Heading:
			name := strings.TrimSpace(string(node.Text(src)))
			if name == "" {
				name = "(empty heading)"
			}
			baseAnchor := anchorFor(name)
			if baseAnchor == "" {
				baseAnchor = "section"
			}
			anchorCounts[baseAnchor]++
			qualifiedAnchor := baseAnchor
			if anchorCounts[baseAnchor] > 1 {
				qualifiedAnchor = fmt.Sprintf("%s-%d", baseAnchor, anchorCounts[baseAnchor]-1)
			}
			lineRange := blockRange(src, node)
			headings = append(headings, headingInfo{
				level:     node.Level,
				name:      name,
				anchor:    baseAnchor,
				qualified: p + "#" + qualifiedAnchor,
				location:  Location{URI: p, Range: lineRange},
				selection: lineRange,
			})
		case *goldast.Link:
			destination := string(node.Destination)
			if strings.HasPrefix(destination, "#") {
				links = append(links, linkInfo{destination: strings.TrimPrefix(destination, "#"), location: Location{URI: p, Range: nodeRange(src, node)}})
			}
		}
		return goldast.WalkContinue, nil
	})
	for i := range headings {
		section := headings[i].location.Range
		section.End = positionForOffset(src, len(src))
		for j := i + 1; j < len(headings); j++ {
			if headings[j].level <= headings[i].level {
				section.End = headings[j].location.Range.Start
				break
			}
		}
		headings[i].sectionRange = section
	}
	return markdownFile{path: p, src: src, headings: headings, links: links}
}

func documentSymbol(p string, src []byte) Symbol {
	return Symbol{
		ID:             SymbolID(p + ":file"),
		Language:       Markdown,
		Kind:           SymbolFile,
		Name:           filepath.Base(p),
		QualifiedName:  p,
		UnitID:         p,
		Location:       Location{URI: p, Range: Range{Start: Position{Line: 1, Column: 1, Offset: 0}, End: positionForOffset(src, len(src))}},
		SelectionRange: Range{Start: Position{Line: 1, Column: 1, Offset: 0}, End: Position{Line: 1, Column: 1, Offset: 0}},
		Backend:        backendInfo(),
	}
}

func headingSymbol(p string, heading headingInfo) Symbol {
	return Symbol{
		ID:             SymbolID(heading.qualified),
		Language:       Markdown,
		Kind:           SymbolNamespace,
		Name:           heading.name,
		QualifiedName:  heading.qualified,
		UnitID:         p,
		Location:       Location{URI: p, Range: heading.sectionRange},
		SelectionRange: heading.selection,
		Tags:           map[string]string{"level": fmt.Sprintf("%d", heading.level), "anchor": heading.anchor},
		Backend:        backendInfo(),
	}
}

func addSymbol(idx *index, sym Symbol) {
	idx.symbols = append(idx.symbols, sym)
	idx.byID[sym.ID] = sym
	idx.byName[sym.Name] = append(idx.byName[sym.Name], sym)
	idx.byName[sym.QualifiedName] = append(idx.byName[sym.QualifiedName], sym)
	if anchor := sym.Tags["anchor"]; anchor != "" {
		idx.byName[anchor] = append(idx.byName[anchor], sym)
	}
	idx.occurrences = append(idx.occurrences, Occurrence{
		SymbolID: sym.ID,
		Kind:     OccurrenceDeclaration,
		Name:     sym.Name,
		Location: Location{URI: sym.Location.URI, Range: sym.SelectionRange},
	})
}

func backendInfo() BackendInfo {
	return BackendInfo{Language: Markdown, Name: "goldmark", ResolutionMode: "structural", Complete: true}
}

func isMarkdownPath(p string) bool {
	ext := strings.ToLower(filepath.Ext(p))
	return ext == ".md" || ext == ".markdown"
}

func blockRange(src []byte, n goldast.Node) Range {
	lines := n.Lines()
	if lines != nil && lines.Len() > 0 {
		segment := lines.At(0)
		start := segment.Start
		end := segment.Stop
		for i := 1; i < lines.Len(); i++ {
			next := lines.At(i)
			if next.Start < start {
				start = next.Start
			}
			if next.Stop > end {
				end = next.Stop
			}
		}
		return Range{Start: positionForOffset(src, start), End: positionForOffset(src, end)}
	}
	return Range{Start: positionForOffset(src, 0), End: positionForOffset(src, len(src))}
}

func nodeRange(src []byte, n goldast.Node) Range {
	if n.Type() == goldast.TypeBlock {
		return blockRange(src, n)
	}
	start := n.Pos()
	if start < 0 {
		start = 0
	}
	end := start + len(n.Text(src))
	if end < start || end > len(src) {
		end = start
	}
	return Range{Start: positionForOffset(src, start), End: positionForOffset(src, end)}
}

func positionForOffset(src []byte, offset int) Position {
	if offset < 0 {
		offset = 0
	}
	if offset > len(src) {
		offset = len(src)
	}
	line, column := 1, 1
	for i := 0; i < offset; i++ {
		if src[i] == '\n' {
			line++
			column = 1
			continue
		}
		column++
	}
	return Position{Line: line, Column: column, Offset: offset}
}

func countLines(src []byte) int {
	if len(src) == 0 {
		return 0
	}
	n := 1
	for _, b := range src {
		if b == '\n' {
			n++
		}
	}
	return n
}

func anchorFor(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case unicode.IsSpace(r) || r == '-':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func sortSymbols(symbols []Symbol) {
	sort.SliceStable(symbols, func(i, j int) bool {
		if symbols[i].Location.URI != symbols[j].Location.URI {
			return symbols[i].Location.URI < symbols[j].Location.URI
		}
		return symbols[i].Location.Range.Start.Offset < symbols[j].Location.Range.Start.Offset
	})
}

func sortOccurrences(occurrences []Occurrence) {
	sort.SliceStable(occurrences, func(i, j int) bool {
		if occurrences[i].Location.URI != occurrences[j].Location.URI {
			return occurrences[i].Location.URI < occurrences[j].Location.URI
		}
		return occurrences[i].Location.Range.Start.Offset < occurrences[j].Location.Range.Start.Offset
	})
}
