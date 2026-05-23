package chatagent

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ProjectReader exposes bounded, read-only views of a project root.
type ProjectReader struct {
	Root           string
	MaxFileBytes   int64
	MaxListEntries int
	MaxMatches     int
	DeniedPrefixes []string
}

const (
	defaultMaxFileBytes   int64 = 256 * 1024
	defaultMaxListEntries       = 200
	defaultMaxMatches           = 100
)

var defaultDeniedPrefixes = []string{".git", ".agentflow/runs"}

var ErrPathEscapesRoot = errors.New("project: path escapes project root")
var ErrPathDenied = errors.New("project: path is denied")

type ProjectEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size,omitempty"`
}

type ProjectFile struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	Size      int64  `json:"size"`
	Truncated bool   `json:"truncated"`
}

type SearchMatch struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

func NewProjectReader(root string) *ProjectReader {
	return &ProjectReader{
		Root:           root,
		MaxFileBytes:   defaultMaxFileBytes,
		MaxListEntries: defaultMaxListEntries,
		MaxMatches:     defaultMaxMatches,
		DeniedPrefixes: append([]string(nil), defaultDeniedPrefixes...),
	}
}

func (r *ProjectReader) List(rel string) ([]ProjectEntry, error) {
	abs, relClean, err := r.resolve(rel)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", relClean, err)
	}
	max := r.MaxListEntries
	if max <= 0 {
		max = defaultMaxListEntries
	}
	out := make([]ProjectEntry, 0, len(entries))
	for _, e := range entries {
		childRel := joinRel(relClean, e.Name())
		if r.isDenied(childRel) {
			continue
		}
		size := int64(0)
		if info, infoErr := e.Info(); infoErr == nil {
			size = info.Size()
		}
		out = append(out, ProjectEntry{
			Name:  e.Name(),
			Path:  childRel,
			IsDir: e.IsDir(),
			Size:  size,
		})
		if len(out) >= max {
			break
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func (r *ProjectReader) Read(rel string) (ProjectFile, error) {
	abs, relClean, err := r.resolve(rel)
	if err != nil {
		return ProjectFile{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return ProjectFile{}, fmt.Errorf("stat %s: %w", relClean, err)
	}
	if info.IsDir() {
		return ProjectFile{}, fmt.Errorf("read %s: is a directory", relClean)
	}
	max := r.MaxFileBytes
	if max <= 0 {
		max = defaultMaxFileBytes
	}
	f, err := os.Open(abs)
	if err != nil {
		return ProjectFile{}, fmt.Errorf("open %s: %w", relClean, err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return ProjectFile{}, fmt.Errorf("read %s: %w", relClean, err)
	}
	truncated := int64(len(data)) > max
	if truncated {
		data = data[:max]
	}
	return ProjectFile{
		Path:      relClean,
		Content:   string(data),
		Size:      info.Size(),
		Truncated: truncated,
	}, nil
}

func (r *ProjectReader) Search(rel, needle string) ([]SearchMatch, error) {
	if strings.TrimSpace(needle) == "" {
		return nil, errors.New("search: needle is required")
	}
	abs, relClean, err := r.resolve(rel)
	if err != nil {
		return nil, err
	}
	max := r.MaxMatches
	if max <= 0 {
		max = defaultMaxMatches
	}
	maxBytes := r.MaxFileBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxFileBytes
	}
	rootAbs, err := filepath.Abs(r.Root)
	if err != nil {
		return nil, fmt.Errorf("search %s: %w", relClean, err)
	}
	var matches []SearchMatch
	walkErr := filepath.WalkDir(abs, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		childRel, err := filepath.Rel(rootAbs, path)
		if err != nil {
			return nil
		}
		childRel = filepath.ToSlash(childRel)
		if childRel == "." {
			childRel = ""
		}
		if r.isDenied(childRel) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > maxBytes {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for i, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, needle) {
				matches = append(matches, SearchMatch{
					Path: childRel,
					Line: i + 1,
					Text: truncateLine(line),
				})
				if len(matches) >= max {
					return fs.SkipAll
				}
			}
		}
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, fs.SkipAll) {
		return nil, fmt.Errorf("search %s: %w", relClean, walkErr)
	}
	return matches, nil
}

func (r *ProjectReader) resolve(rel string) (string, string, error) {
	if strings.TrimSpace(r.Root) == "" {
		return "", "", errors.New("project: root is not configured")
	}
	cleaned := filepath.ToSlash(filepath.Clean("/" + strings.TrimSpace(rel)))
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "." {
		cleaned = ""
	}
	if r.isDenied(cleaned) {
		return "", cleaned, ErrPathDenied
	}
	rootAbs, err := filepath.Abs(r.Root)
	if err != nil {
		return "", cleaned, fmt.Errorf("resolve root: %w", err)
	}
	abs := filepath.Join(rootAbs, filepath.FromSlash(cleaned))
	absClean, err := filepath.Abs(abs)
	if err != nil {
		return "", cleaned, fmt.Errorf("resolve %s: %w", cleaned, err)
	}
	if absClean != rootAbs && !strings.HasPrefix(absClean, rootAbs+string(filepath.Separator)) {
		return "", cleaned, ErrPathEscapesRoot
	}
	return absClean, cleaned, nil
}

func (r *ProjectReader) isDenied(rel string) bool {
	if rel == "" {
		return false
	}
	for _, p := range r.DeniedPrefixes {
		p = strings.TrimSuffix(p, "/")
		if p == "" {
			continue
		}
		if rel == p || strings.HasPrefix(rel, p+"/") {
			return true
		}
	}
	return false
}

func joinRel(parent, child string) string {
	if parent == "" {
		return child
	}
	return parent + "/" + child
}

func truncateLine(line string) string {
	const max = 240
	if len(line) <= max {
		return line
	}
	return line[:max] + "..."
}
