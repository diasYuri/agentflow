package web

import (
	"embed"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// AssetProvider abstracts how the server serves the static HTML shell
// and bundled assets. The default implementation reads from an embedded
// filesystem; the `--dev-assets` mode swaps in a directory provider so
// frontend authors can iterate without rebuilding the Go binary.
type AssetProvider interface {
	ServeShell(w http.ResponseWriter, r *http.Request)
	ServeAsset(w http.ResponseWriter, r *http.Request, name string)
}

//go:embed assets/shell.html
var embeddedShell []byte

//go:embed assets/static
var embeddedStatic embed.FS

// NewEmbeddedAssets returns the production asset provider backed by the
// `go:embed` artefacts in this package.
func NewEmbeddedAssets() AssetProvider {
	staticFS, err := fs.Sub(embeddedStatic, "assets/static")
	if err != nil {
		staticFS = emptyFS{}
	}
	return &embeddedAssets{shell: embeddedShell, static: http.FS(staticFS)}
}

// NewDevAssets returns an AssetProvider that serves files directly from
// dir. The directory must contain an index.html and a static/
// subdirectory; missing files fall back to the embedded copy so a
// running dev server still has a usable shell.
func NewDevAssets(dir string) AssetProvider {
	return &devAssets{dir: dir, fallback: NewEmbeddedAssets()}
}

type embeddedAssets struct {
	shell  []byte
	static http.FileSystem
}

func (e *embeddedAssets) ServeShell(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(e.shell)
}

func (e *embeddedAssets) ServeAsset(w http.ResponseWriter, r *http.Request, name string) {
	clean := path.Clean("/" + name)
	if clean == "/" || strings.HasPrefix(clean, "/..") {
		http.NotFound(w, r)
		return
	}
	file, err := e.static.Open(strings.TrimPrefix(clean, "/"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	http.ServeContent(w, r, info.Name(), info.ModTime(), file)
}

type devAssets struct {
	dir      string
	fallback AssetProvider
}

func (d *devAssets) ServeShell(w http.ResponseWriter, r *http.Request) {
	candidate := filepath.Join(d.dir, "index.html")
	if data, err := os.ReadFile(candidate); err == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(data)
		return
	}
	d.fallback.ServeShell(w, r)
}

func (d *devAssets) ServeAsset(w http.ResponseWriter, r *http.Request, name string) {
	clean := path.Clean("/" + name)
	if clean == "/" || strings.HasPrefix(clean, "/..") {
		http.NotFound(w, r)
		return
	}
	for _, assetDir := range []string{"static", "assets"} {
		candidate := filepath.Join(d.dir, assetDir, filepath.FromSlash(strings.TrimPrefix(clean, "/")))
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			http.ServeFile(w, r, candidate)
			return
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	d.fallback.ServeAsset(w, r, name)
}

type emptyFS struct{}

func (emptyFS) Open(string) (fs.File, error) { return nil, os.ErrNotExist }
