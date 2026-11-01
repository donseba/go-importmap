package library

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"testing/iotest"
)

// writeFixture writes content to root/name.
func writeFixture(t *testing.T, root, name, content string) {
	t.Helper()

	filename := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// readFixture returns the content of root/name.
func readFixture(t *testing.T, root, name string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}

	return string(data)
}

// entries returns the names in dir, so a test can see that no temporary
// file was left behind.
func entries(t *testing.T, dir string) []string {
	t.Helper()

	list, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	var names []string
	for _, entry := range list {
		names = append(names, entry.Name())
	}

	return names
}

// contentServer serves files by path and answers 404 for any other path.
func contentServer(t *testing.T, files map[string]string) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}

		_, _ = w.Write([]byte(content))
	}))
	t.Cleanup(server.Close)

	return server
}

// requireChmod skips the test when file permissions do not restrict the test
// process, as for root or on Windows.
func requireChmod(t *testing.T) {
	t.Helper()

	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("file permissions do not restrict this process")
	}
}

// chmod changes the mode of name and restores it when the test ends, so
// t.TempDir can remove it.
func chmod(t *testing.T, name string, mode os.FileMode, restore os.FileMode) {
	t.Helper()

	if err := os.Chmod(name, mode); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_ = os.Chmod(name, restore)
	})
}

// cancelReader cancels its context on the first read and then ends the data,
// as when a request is cancelled while the body is being copied.
type cancelReader struct {
	cancel context.CancelFunc
}

func (r cancelReader) Read(p []byte) (int, error) {
	r.cancel()

	return copy(p, "partial"), io.EOF
}

func TestIncludesGetAndCheck(t *testing.T) {
	includes := Includes{{File: "dist/[a.js"}, {File: "dist/**.js", As: "app"}}

	// The invalid pattern matches nothing; the next one still does.
	if got := includes.Get("dist/app.js"); got == nil || got.As != "app" {
		t.Fatalf("Get = %+v, want the dist/**.js include", got)
	}

	err := includes.Check()
	if err == nil || !strings.Contains(err.Error(), `"dist/[a.js"`) {
		t.Fatalf("Check = %v, want an error naming the invalid pattern", err)
	}

	if err := (Includes{{File: "dist/**.js"}, {Raw: "https://cdn.example/x.js"}}).Check(); err != nil {
		t.Fatalf("Check of valid patterns = %v", err)
	}
}

func TestIncludesGetMatchesWholePath(t *testing.T) {
	includes := Includes{{File: "dist/**.js", As: "app"}, {File: "/css/site.css", As: "site"}}

	tests := []struct {
		path string
		want string
	}{
		{"dist/app.js", "app"},
		{"dist/js/nested/app.js", "app"},
		{"css/site.css", "site"},
		{"dist/app.css", ""},
		{"vendor/dist/app.js", ""},
		{"css/site.min.css", ""},
	}

	for _, tt := range tests {
		got := includes.Get(tt.path)
		if tt.want == "" {
			if got != nil {
				t.Errorf("Get(%q) = %+v, want nil", tt.path, got)
			}

			continue
		}

		if got == nil || got.As != tt.want {
			t.Errorf("Get(%q) = %+v, want the %q include", tt.path, got, tt.want)
		}
	}
}

func TestIncludeName(t *testing.T) {
	tests := []struct {
		include Include
		want    string
	}{
		{Include{File: "dist/htmx.min.js", As: "htmx-core"}, "htmx-core"},
		{Include{File: "dist/htmx.min.js"}, "htmx"},
		{Include{File: "/ext/json-enc.js"}, "json-enc"},
		{Include{File: "js/*bootstrap.bundle.js"}, "bootstrap"},
		{Include{File: "LICENSE"}, "LICENSE"},
	}

	for _, tt := range tests {
		if got := tt.include.Name(); got != tt.want {
			t.Errorf("Name of %+v = %q, want %q", tt.include, got, tt.want)
		}
	}
}

func TestPackageCacheDir(t *testing.T) {
	tests := []struct {
		pkg  Package
		want string
	}{
		{Package{Name: "htmx", Version: "4.0.0"}, "cache/htmx/4.0.0"},
		{Package{Name: "htmx"}, "cache/htmx/latest"},
		{Package{Name: "@hotwired/turbo", Version: "8.0.13"}, "cache/@hotwired/turbo/8.0.13"},
	}

	for _, tt := range tests {
		if got := tt.pkg.CacheDir("cache"); got != tt.want {
			t.Errorf("CacheDir of %s@%s = %q, want %q", tt.pkg.Name, tt.pkg.Version, got, tt.want)
		}
	}
}

func TestPackageHasCache(t *testing.T) {
	root := t.TempDir()
	pkg := Package{Name: "htmx", Version: "4.0.0"}

	if pkg.HasCache(root, "cache") {
		t.Fatal("HasCache = true before anything was cached")
	}

	writeFixture(t, root, "cache/htmx/4.0.0/ext/json-enc.js", "ext")

	if !pkg.HasCache(root, "cache") {
		t.Fatal("HasCache = false after the version was cached")
	}

	other := Package{Name: "htmx", Version: "4.0.1"}
	if other.HasCache(root, "cache") {
		t.Fatal("HasCache = true for a version that was not cached")
	}

	tests := []struct {
		filePath string
		want     bool
	}{
		{"ext/json-enc.js", true},
		{"ext", false},
		{"htmx.min.js", false},
	}

	for _, tt := range tests {
		if got := pkg.HasCacheFile(root, "cache", tt.filePath); got != tt.want {
			t.Errorf("HasCacheFile(%q) = %v, want %v", tt.filePath, got, tt.want)
		}
	}
}

func TestPackageHasAssets(t *testing.T) {
	root := t.TempDir()
	pkg := Package{Name: "htmx", Version: "4.0.0"}

	if got := pkg.AssetsDir("assets"); got != "assets/htmx" {
		t.Fatalf("AssetsDir = %q, want assets/htmx without the version", got)
	}

	if pkg.HasAssets(root, "assets") || pkg.HasAssetFile(root, "assets", "htmx.min.js") {
		t.Fatal("HasAssets or HasAssetFile = true before anything was written")
	}

	writeFixture(t, root, "assets/htmx/htmx.min.js", "htmx")

	if !pkg.HasAssets(root, "assets") {
		t.Error("HasAssets = false after the assets were written")
	}

	if !pkg.HasAssetFile(root, "assets", "htmx.min.js") {
		t.Error("HasAssetFile = false for a written file")
	}

	if pkg.HasAssetFile(root, "assets", "ext/json-enc.js") {
		t.Error("HasAssetFile = true for a file that was not written")
	}
}

func TestPackageAssetsAt(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "assets/htmx/htmx.min.js", "htmx")
	writeFixture(t, root, "assets/htmx/htmx.css", "css")
	writeFixture(t, root, "assets/htmx/ext/json-enc.js", "ext")
	pkg := Package{Name: "htmx", Version: "4.0.0"}

	files, dir, err := pkg.AssetsAt(root, "assets", "")
	if err != nil {
		t.Fatal(err)
	}

	want := Files{
		{Path: "/assets/htmx/ext/json-enc.js", LocalPath: "ext/json-enc.js", Type: FileTypeJS},
		{Path: "/assets/htmx/htmx.css", LocalPath: "htmx.css", Type: FileTypeCSS},
		{Path: "/assets/htmx/htmx.min.js", LocalPath: "htmx.min.js", Type: FileTypeJS},
	}
	if dir != "assets/htmx" || !slices.Equal(files, want) {
		t.Fatalf("AssetsAt = %q, %+v, want assets/htmx, %+v", dir, files, want)
	}

	// A leading slash on the file path is ignored.
	files, _, err = pkg.AssetsAt(root, "assets", "/ext")
	if err != nil {
		t.Fatal(err)
	}

	want = Files{{Path: "/assets/htmx/ext/json-enc.js", LocalPath: "ext/json-enc.js", Type: FileTypeJS}}
	if !slices.Equal(files, want) {
		t.Fatalf("AssetsAt of /ext = %+v, want %+v", files, want)
	}
}

func TestPackageAssetsAtErrors(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "assets/htmx/htmx.min.js", "htmx")
	pkg := Package{Name: "htmx"}

	tests := []struct {
		filePath string
		want     string
	}{
		{"ext", "does not exist"},
		{"htmx.min.js", "failed to read directory"},
	}

	for _, tt := range tests {
		files, dir, err := pkg.AssetsAt(root, "assets", tt.filePath)
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("AssetsAt(%q) error = %v, want one containing %q", tt.filePath, err, tt.want)
		}

		if files != nil || dir != "" {
			t.Errorf("AssetsAt(%q) = %q, %+v with an error, want nothing", tt.filePath, dir, files)
		}
	}
}

func TestPackageAssetsRelativeToWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	writeFixture(t, root, "public/htmx/htmx.min.js", "htmx")
	pkg := Package{Name: "htmx"}

	files, dir, err := pkg.Assets("public", "")
	if err != nil {
		t.Fatal(err)
	}

	want := Files{{Path: "/public/htmx/htmx.min.js", LocalPath: "htmx.min.js", Type: FileTypeJS}}
	if dir != "public/htmx" || !slices.Equal(files, want) {
		t.Fatalf("Assets = %q, %+v, want public/htmx, %+v", dir, files, want)
	}
}

func TestPackageAssetsUnreadableSubdirectory(t *testing.T) {
	requireChmod(t)

	root := t.TempDir()
	writeFixture(t, root, "assets/htmx/htmx.min.js", "htmx")
	writeFixture(t, root, "assets/htmx/ext/json-enc.js", "ext")
	chmod(t, filepath.Join(root, "assets", "htmx", "ext"), 0, 0o755)
	pkg := Package{Name: "htmx"}

	files, _, err := pkg.AssetsAt(root, "assets", "")
	if err == nil || !strings.Contains(err.Error(), "failed to read directory") || !strings.Contains(err.Error(), "ext") {
		t.Fatalf("AssetsAt error = %v, want one naming the unreadable directory", err)
	}

	if files != nil {
		t.Fatalf("AssetsAt = %+v with an error, want no files", files)
	}
}

func TestPackageCacheFiles(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "cache/htmx/4.0.0/htmx.min.js", "htmx")
	writeFixture(t, root, "cache/htmx/4.0.0/ext/json-enc.js", "ext")
	pkg := Package{Name: "htmx", Version: "4.0.0"}

	files, err := pkg.CacheFiles(root, "cache")
	if err != nil {
		t.Fatal(err)
	}

	want := Files{
		{Path: "/cache/htmx/4.0.0/ext/json-enc.js", LocalPath: "ext/json-enc.js", Type: FileTypeJS},
		{Path: "/cache/htmx/4.0.0/htmx.min.js", LocalPath: "htmx.min.js", Type: FileTypeJS},
	}
	if !slices.Equal(files, want) {
		t.Fatalf("CacheFiles = %+v, want %+v", files, want)
	}

	other := Package{Name: "htmx", Version: "4.0.1"}
	if _, err := other.CacheFiles(root, "cache"); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("CacheFiles of an uncached version error = %v, want does not exist", err)
	}
}

func TestPackageMakeCache(t *testing.T) {
	server := contentServer(t, map[string]string{"/htmx.min.js": "window.htmx = {}"})
	root := t.TempDir()
	pkg := Package{Name: "htmx", Version: "4.0.0"}

	if err := pkg.MakeCache(root, "cache", "dist/htmx.min.js", server.URL+"/htmx.min.js"); err != nil {
		t.Fatal(err)
	}

	if got := readFixture(t, root, "cache/htmx/4.0.0/dist/htmx.min.js"); got != "window.htmx = {}" {
		t.Fatalf("cached file = %q", got)
	}

	info, err := os.Stat(filepath.Join(root, "cache", "htmx", "4.0.0", "dist", "htmx.min.js"))
	if err != nil {
		t.Fatal(err)
	}

	if info.Mode().Perm() != 0o644 {
		t.Errorf("cached file mode = %v, want 0644", info.Mode().Perm())
	}

	if got := entries(t, filepath.Join(root, "cache", "htmx", "4.0.0", "dist")); !slices.Equal(got, []string{"htmx.min.js"}) {
		t.Errorf("cache directory holds %v, want only htmx.min.js", got)
	}

	if !pkg.HasCacheFile(root, "cache", "dist/htmx.min.js") {
		t.Error("HasCacheFile = false after MakeCache")
	}
}

func TestPackageMakeAssets(t *testing.T) {
	server := contentServer(t, map[string]string{
		"/htmx.min.js": "remote htmx",
		"/json-enc.js": "remote json-enc",
	})
	root := t.TempDir()
	pkg := Package{Name: "htmx", Version: "4.0.0"}
	writeFixture(t, root, "cache/htmx/4.0.0/htmx.min.js", "cached htmx")

	// A source that cannot be downloaded shows the cache file is copied.
	if err := pkg.MakeAssets(root, "cache", "assets", "htmx.min.js", "ftp://example.invalid/htmx.min.js"); err != nil {
		t.Fatal(err)
	}

	if got := readFixture(t, root, "assets/htmx/htmx.min.js"); got != "cached htmx" {
		t.Errorf("asset copied from the cache = %q", got)
	}

	// A file that is not cached is downloaded.
	if err := pkg.MakeAssets(root, "cache", "assets", "ext/json-enc.js", server.URL+"/json-enc.js"); err != nil {
		t.Fatal(err)
	}

	if got := readFixture(t, root, "assets/htmx/ext/json-enc.js"); got != "remote json-enc" {
		t.Errorf("downloaded asset = %q", got)
	}

	// Without a cache directory the cache is not looked at, even when the
	// file exists where an empty cache directory would put it.
	writeFixture(t, root, "htmx/4.0.0/htmx.min.js", "stray htmx")

	if err := pkg.MakeAssetsContext(t.Context(), root, "", "public", "htmx.min.js", server.URL+"/htmx.min.js"); err != nil {
		t.Fatal(err)
	}

	if got := readFixture(t, root, "public/htmx/htmx.min.js"); got != "remote htmx" {
		t.Errorf("asset without a cache directory = %q, want the download", got)
	}
}

func TestPackageMakeAssetsUnreadableCache(t *testing.T) {
	requireChmod(t)

	root := t.TempDir()
	writeFixture(t, root, "cache/htmx/4.0.0/htmx.min.js", "cached htmx")
	chmod(t, filepath.Join(root, "cache", "htmx", "4.0.0", "htmx.min.js"), 0, 0o644)
	pkg := Package{Name: "htmx", Version: "4.0.0"}

	err := pkg.MakeAssets(root, "cache", "assets", "htmx.min.js", "ftp://example.invalid/htmx.min.js")
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("MakeAssets error = %v, want a permission error", err)
	}

	if pkg.HasAssetFile(root, "assets", "htmx.min.js") {
		t.Fatal("MakeAssets wrote an asset although the cache could not be read")
	}
}

func TestDownloadFileErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusGone)
	}))
	defer server.Close()

	tests := []struct {
		src  string
		want string
	}{
		{"http://example.invalid/\x7f.js", "invalid control character"},
		{"ftp://example.invalid/htmx.min.js", "unsupported protocol scheme"},
		{server.URL + "/htmx.min.js", "HTTP 410 Gone"},
	}

	for _, tt := range tests {
		dir := t.TempDir()

		err := downloadFile(t.Context(), filepath.Join(dir, "htmx.min.js"), tt.src)
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("downloadFile(%q) error = %v, want one containing %q", tt.src, err, tt.want)
		}

		if got := entries(t, dir); len(got) != 0 {
			t.Errorf("downloadFile(%q) left %v behind", tt.src, got)
		}
	}
}

func TestDownloadFileIncompleteBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1024")
		_, _ = w.Write([]byte("incomplete"))
	}))
	defer server.Close()

	dir := t.TempDir()

	err := downloadFile(t.Context(), filepath.Join(dir, "htmx.min.js"), server.URL)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("downloadFile error = %v, want an unexpected EOF", err)
	}

	if got := entries(t, dir); len(got) != 0 {
		t.Fatalf("incomplete download left %v behind", got)
	}
}

func TestWriteFileCancelledBeforeWriting(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := writeFile(ctx, filepath.Join(root, "dist", "htmx.min.js"), strings.NewReader("htmx"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("writeFile error = %v, want context.Canceled", err)
	}

	if got := entries(t, root); len(got) != 0 {
		t.Fatalf("cancelled writeFile created %v", got)
	}
}

func TestWriteFileCancelledWhileCopying(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	err := writeFile(ctx, filepath.Join(root, "htmx.min.js"), cancelReader{cancel: cancel})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("writeFile error = %v, want context.Canceled", err)
	}

	if got := entries(t, root); len(got) != 0 {
		t.Fatalf("cancelled writeFile left %v behind", got)
	}
}

func TestWriteFileReadError(t *testing.T) {
	root := t.TempDir()
	errRead := errors.New("connection reset")

	err := writeFile(t.Context(), filepath.Join(root, "htmx.min.js"), iotest.ErrReader(errRead))
	if !errors.Is(err, errRead) {
		t.Fatalf("writeFile error = %v, want %v", err, errRead)
	}

	if got := entries(t, root); len(got) != 0 {
		t.Fatalf("failed writeFile left %v behind", got)
	}
}

func TestWriteFileParentIsAFile(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "dist", "not a directory")

	if err := writeFile(t.Context(), filepath.Join(root, "dist", "htmx.min.js"), strings.NewReader("htmx")); err == nil {
		t.Fatal("writeFile below a file succeeded")
	}

	if got := readFixture(t, root, "dist"); got != "not a directory" {
		t.Fatalf("writeFile changed the file in its way to %q", got)
	}
}

func TestWriteFileUnwritableDirectory(t *testing.T) {
	requireChmod(t)

	root := t.TempDir()
	dir := filepath.Join(root, "dist")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	chmod(t, dir, 0o555, 0o755)

	err := writeFile(t.Context(), filepath.Join(dir, "htmx.min.js"), strings.NewReader("htmx"))
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("writeFile error = %v, want a permission error", err)
	}
}

func TestWriteFileOntoDirectory(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "htmx.min.js/keep.txt", "keep")

	if err := writeFile(t.Context(), filepath.Join(root, "htmx.min.js"), strings.NewReader("htmx")); err == nil {
		t.Fatal("writeFile replaced a directory")
	}

	if got := entries(t, root); !slices.Equal(got, []string{"htmx.min.js"}) {
		t.Fatalf("failed rename left %v behind", got)
	}

	if got := readFixture(t, root, "htmx.min.js/keep.txt"); got != "keep" {
		t.Fatalf("directory content = %q after the failed write", got)
	}
}
