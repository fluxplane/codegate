package agentruntime

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/codewandler/codegate"
)

const (
	defaultWalkDepth   = 50
	defaultMaxEntries  = 10000
	defaultReadMaxByte = int64(0)
)

// ReadFileFunc adapts agentruntime Workspace.ReadFile without coupling this
// module to agentruntime's concrete ResolvedPath type.
type ReadFileFunc func(ctx context.Context, filePath string, maxBytes int64) ([]byte, bool, error)

// ListFilesFunc lists workspace-relative files for an editor scope.
type ListFilesFunc func(ctx context.Context, scope codegate.Scope) ([]string, error)

// WalkFunc adapts agentruntime Workspace.Walk by returning only the fields the
// editor needs.
type WalkFunc func(ctx context.Context, root string, opts WalkOptions) ([]WalkEntry, bool, error)

// WalkOptions mirrors the agentruntime workspace traversal knobs used by the
// editor source adapter.
type WalkOptions struct {
	Depth         int
	ShowHidden    bool
	MaxEntries    int
	FilesOnly     bool
	SkipDirs      []string
	FilterPattern string
}

// WalkEntry is a workspace-relative walk result.
type WalkEntry struct {
	Path string
	Kind string
}

type Source struct {
	read       ReadFileFunc
	list       ListFilesFunc
	maxBytes   int64
	walkDepth  int
	maxEntries int
	showHidden bool
	skipDirs   []string
}

type Option func(*Source)

// NewSource adapts explicit read and list callbacks into a codegate Source.
func NewSource(read ReadFileFunc, list ListFilesFunc, opts ...Option) (*Source, error) {
	if read == nil {
		return nil, errors.New("agentruntime adapter: nil read function")
	}
	if list == nil {
		return nil, errors.New("agentruntime adapter: nil list function")
	}
	source := &Source{
		read:       read,
		list:       list,
		maxBytes:   defaultReadMaxByte,
		walkDepth:  defaultWalkDepth,
		maxEntries: defaultMaxEntries,
	}
	for _, opt := range opts {
		opt(source)
	}
	return source, nil
}

// NewWalkSource adapts a read callback plus a workspace walk callback into a
// codegate Source.
func NewWalkSource(read ReadFileFunc, walk WalkFunc, opts ...Option) (*Source, error) {
	if walk == nil {
		return nil, errors.New("agentruntime adapter: nil walk function")
	}
	source, err := NewSource(read, nilList, opts...)
	if err != nil {
		return nil, err
	}
	source.list = source.walkListFiles(walk)
	return source, nil
}

// WithMaxBytes sets the maximum bytes requested from the read callback.
func WithMaxBytes(maxBytes int64) Option {
	return func(source *Source) {
		source.maxBytes = maxBytes
	}
}

// WithWalkLimits sets depth and entry limits used by NewWalkSource.
func WithWalkLimits(depth, maxEntries int) Option {
	return func(source *Source) {
		source.walkDepth = depth
		source.maxEntries = maxEntries
	}
}

// WithShowHidden controls whether NewWalkSource asks the walk callback to
// include hidden files.
func WithShowHidden(show bool) Option {
	return func(source *Source) {
		source.showHidden = show
	}
}

// WithSkipDirs configures directory names skipped by NewWalkSource.
func WithSkipDirs(skipDirs ...string) Option {
	return func(source *Source) {
		source.skipDirs = cleanList(skipDirs)
	}
}

func (s *Source) ListFiles(ctx context.Context, scope codegate.Scope) ([]string, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	files, err := s.list(ctx, scope)
	if err != nil {
		return nil, err
	}
	return normalizeFiles(files), nil
}

func (s *Source) ReadFile(ctx context.Context, filePath string) ([]byte, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	data, truncated, err := s.read(ctx, cleanPath(filePath), s.maxBytes)
	if err != nil {
		return nil, err
	}
	if truncated {
		return nil, fmt.Errorf("agentruntime adapter: %s exceeds read limit", cleanPath(filePath))
	}
	return append([]byte(nil), data...), nil
}

func (s *Source) walkListFiles(walk WalkFunc) ListFilesFunc {
	return func(ctx context.Context, scope codegate.Scope) ([]string, error) {
		root := cleanPath(firstNonEmpty(scope.Path, scope.Root, "."))
		entries, truncated, err := walk(ctx, root, WalkOptions{
			Depth:      s.walkDepth,
			ShowHidden: s.showHidden,
			MaxEntries: s.maxEntries,
			FilesOnly:  true,
			SkipDirs:   append([]string(nil), s.skipDirs...),
		})
		if err != nil {
			return nil, err
		}
		if truncated {
			return nil, fmt.Errorf("agentruntime adapter: workspace walk truncated at %d entries", s.maxEntries)
		}
		files := make([]string, 0, len(entries))
		for _, entry := range entries {
			if entry.Kind != "" && entry.Kind != "file" {
				continue
			}
			filePath := cleanPath(entry.Path)
			if inScope(filePath, root) {
				files = append(files, filePath)
			}
		}
		return files, nil
	}
}

func nilList(context.Context, codegate.Scope) ([]string, error) {
	return nil, errors.New("agentruntime adapter: nil list function")
}

func normalizeFiles(files []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(files))
	for _, file := range files {
		file = cleanPath(file)
		if file == "" || seen[file] {
			continue
		}
		seen[file] = true
		out = append(out, file)
	}
	sort.Strings(out)
	return out
}

func cleanList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = cleanPath(value)
		if value != "" && value != "." {
			out = append(out, value)
		}
	}
	return out
}

func cleanPath(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" {
		return "."
	}
	value = path.Clean(value)
	if value == "/" {
		return "."
	}
	value = strings.TrimPrefix(value, "/")
	if value == "" {
		return "."
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return "."
}

func inScope(filePath, root string) bool {
	return root == "." || filePath == root || strings.HasPrefix(filePath, root+"/")
}
