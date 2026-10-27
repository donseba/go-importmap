package importmap

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/donseba/go-importmap/assets"
	"github.com/donseba/go-importmap/library"
)

// vendorServer serves files by path and counts the requests per path.
func vendorServer(t *testing.T, files map[string]string) (*httptest.Server, func(string) int) {
	t.Helper()

	var (
		mu       sync.Mutex
		requests = map[string]int{}
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests[r.URL.Path]++
		mu.Unlock()

		content, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}

		_, _ = w.Write([]byte(content))
	}))
	t.Cleanup(server.Close)

	return server, func(p string) int {
		mu.Lock()
		defer mu.Unlock()

		return requests[p]
	}
}

func TestVendorWritesFilesLicenceAndLock(t *testing.T) {
	server, requests := vendorServer(t, map[string]string{
		"/htmx/htmx.esm.min.js": "export default {}",
		"/htmx/htmx.min.js":     "window.htmx = {}",
		"/htmx/LICENSE":         "Zero-Clause BSD",
		"/bootstrap/b.min.css":  "body{}",
	})
	provider := fixtureProvider{baseURL: server.URL, paths: map[string][]string{
		"htmx":      {"htmx.esm.min.js", "htmx.min.js", "LICENSE"},
		"bootstrap": {"b.min.css"},
	}}

	dir := t.TempDir()
	im := New().WithProvider(provider).WithPackages([]library.Package{
		{Name: "htmx", Version: "4.0.0", Require: library.Includes{{File: "htmx.esm.min.js", As: "htmx"}}},
		{Name: "bootstrap", Version: "5.3.8", License: server.URL + "/htmx/LICENSE"},
		{Name: "remote", Require: library.Includes{{Raw: "https://cdn.example/x.js", As: "x"}}},
	})

	lock, err := im.Vendor(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, LockFile))
	if err != nil {
		t.Fatal(err)
	}

	var written Lock
	if err := json.Unmarshal(data, &written); err != nil {
		t.Fatal(err)
	}

	if len(written.Packages) != 3 || written.Packages[0].License != "htmx/LICENSE" ||
		written.Packages[1].License != "bootstrap/LICENSE" {
		t.Fatalf("lock = %s", data)
	}

	htmx := lock.Packages[0]
	if len(htmx.Files) != 2 || htmx.Files[1].Path != "htmx/htmx.esm.min.js" || htmx.Files[1].As != "htmx" ||
		htmx.Files[1].Integrity != assets.Integrity([]byte("export default {}")) || htmx.Files[0].As != "" {
		t.Fatalf("htmx files = %+v", htmx.Files)
	}

	if _, err := os.Stat(filepath.Join(dir, "htmx", "htmx.min.js")); !os.IsNotExist(err) {
		t.Errorf("a file the package does not require was vendored: %v", err)
	}

	if remote := lock.Packages[2]; len(remote.Files) != 1 || remote.Files[0].Source != "https://cdn.example/x.js" ||
		remote.Files[0].Path != "" {
		t.Errorf("raw import = %+v", remote)
	}

	// The vendored files load without network.
	loaded, err := FromFS(os.DirFS(dir), LockFile)
	if err != nil {
		t.Fatal(err)
	}

	if got := loaded.Structure.Imports["htmx"]; !strings.HasPrefix(got, "/assets/htmx/htmx.esm.min.js?v=") {
		t.Errorf("htmx URL = %q", got)
	}

	if got := loaded.Structure.Styles["b.min.css"]; !strings.HasPrefix(got, "/assets/bootstrap/b.min.css?v=") {
		t.Errorf("style URL = %q", got)
	}

	if got := loaded.Structure.Imports["x"]; got != "https://cdn.example/x.js" {
		t.Errorf("raw URL = %q", got)
	}

	// A second run downloads nothing that is unchanged.
	if _, err := im.Vendor(t.Context(), dir); err != nil {
		t.Fatal(err)
	}

	if got := requests("/htmx/htmx.esm.min.js"); got != 1 {
		t.Errorf("unchanged file downloaded %d times", got)
	}

	// Files no longer required are removed.
	im.WithPackages(im.packages[:1])

	if _, err := im.Vendor(t.Context(), dir); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "bootstrap", "b.min.css")); !os.IsNotExist(err) {
		t.Errorf("stale file kept: %v", err)
	}
}

// TestVendorRefusesAnInvalidRequirePattern checks that Vendor and Fetch
// stop before anything is downloaded when a package has a pattern that is
// not valid; it was printed and skipped.
func TestVendorRefusesAnInvalidRequirePattern(t *testing.T) {
	server, requests := vendorServer(t, map[string]string{"/good/good.js": "1", "/bad/bad.js": "2"})
	provider := fixtureProvider{baseURL: server.URL, paths: map[string][]string{"good": {"good.js"}, "bad": {"bad.js"}}}

	im := New().WithProvider(provider).WithPackages([]library.Package{
		{Name: "good", Version: "1"},
		{Name: "bad", Version: "1", Require: library.Includes{{File: "[bad.js"}}},
	})

	_, err := im.Vendor(t.Context(), t.TempDir())
	if err == nil || !strings.Contains(err.Error(), `package bad: require pattern "[bad.js"`) {
		t.Fatalf("Vendor: err = %v", err)
	}

	if err := im.Fetch(t.Context()); err == nil || !strings.Contains(err.Error(), "package bad") {
		t.Fatalf("Fetch: err = %v", err)
	}

	if got := requests("/good/good.js"); got != 0 {
		t.Errorf("downloaded good.js %d times before the error", got)
	}
}

func TestVendorRejectsPathsOutsideThePackage(t *testing.T) {
	server, _ := vendorServer(t, map[string]string{"/evil/../x.js": "x"})

	for _, localPath := range []string{"../x.js", "../other/x.js", "a/../../x.js", "/../../x.js"} {
		provider := fixtureProvider{baseURL: server.URL, paths: map[string][]string{"evil": {localPath}}}
		dir := t.TempDir()

		_, err := New().WithProvider(provider).WithPackage(library.Package{Name: "evil", Version: "1"}).Vendor(t.Context(), dir)
		if err == nil || !strings.Contains(err.Error(), "invalid file path") {
			t.Errorf("%s: err = %v", localPath, err)
		}
	}
}

// TestVendorRefusesLockPathsOutsideTheDirectory checks that a hand-edited
// lock cannot make Vendor remove a file outside the vendor directory: such
// a lock is refused before anything is downloaded or removed.
func TestVendorRefusesLockPathsOutsideTheDirectory(t *testing.T) {
	server, requests := vendorServer(t, map[string]string{"/demo/demo.js": "one"})
	provider := fixtureProvider{baseURL: server.URL, paths: map[string][]string{"demo": {"demo.js"}}}

	for _, name := range []string{"../outside.txt", "demo/../../outside.txt", "/outside.txt", `demo\..\..\outside.txt`, "."} {
		parent := t.TempDir()
		dir := filepath.Join(parent, "vendor")

		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}

		outside := filepath.Join(parent, "outside.txt")
		if err := os.WriteFile(outside, []byte("keep"), 0o644); err != nil {
			t.Fatal(err)
		}

		previous := &Lock{LockfileVersion: lockVersion, Packages: []LockedPackage{{
			Name:    "demo",
			Version: "1",
			Files:   []LockedFile{{Path: name, Source: server.URL + "/demo/demo.js", Integrity: "sha384-x"}},
		}}}
		if err := writeLock(dir, previous); err != nil {
			t.Fatal(err)
		}

		_, err := New().WithProvider(provider).WithPackage(library.Package{Name: "demo", Version: "1"}).Vendor(t.Context(), dir)
		if err == nil || !strings.Contains(err.Error(), "outside the vendor directory") {
			t.Errorf("%s: err = %v", name, err)
		}

		if _, err := os.Stat(outside); err != nil {
			t.Errorf("%s: the file outside the vendor directory is gone: %v", name, err)
		}
	}

	if got := requests("/demo/demo.js"); got != 0 {
		t.Errorf("downloaded %d times despite the refused lock", got)
	}
}

// TestVendorLeavesOutHiddenFiles checks that Vendor neither downloads nor
// locks a file with a part that starts with a dot: the assets package does
// not serve it, so FromFS would report it missing.
func TestVendorLeavesOutHiddenFiles(t *testing.T) {
	server, requests := vendorServer(t, map[string]string{
		"/demo/demo.js":        "export default 1",
		"/demo/.hidden.js":     "secret",
		"/demo/.github/x.js":   "ci",
		"/demo/dist/.cache.js": "cache",
	})
	provider := fixtureProvider{baseURL: server.URL, paths: map[string][]string{
		"demo": {"demo.js", ".hidden.js", ".github/x.js", "dist/.cache.js"},
	}}

	dir := t.TempDir()

	lock, err := New().WithProvider(provider).WithPackage(library.Package{Name: "demo", Version: "1"}).Vendor(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}

	if files := lock.Packages[0].Files; len(files) != 1 || files[0].Path != "demo/demo.js" {
		t.Fatalf("locked %+v, want only demo/demo.js", files)
	}

	for _, hidden := range []string{"/demo/.hidden.js", "/demo/.github/x.js", "/demo/dist/.cache.js"} {
		if got := requests(hidden); got != 0 {
			t.Errorf("%s downloaded %d times", hidden, got)
		}
	}

	if _, err := FromFS(os.DirFS(dir), LockFile); err != nil {
		t.Errorf("FromFS of the vendored files: %v", err)
	}
}

func TestVendorKeepsPreviousFilesOnFailure(t *testing.T) {
	server, _ := vendorServer(t, map[string]string{"/demo/demo.js": "one"})
	provider := fixtureProvider{baseURL: server.URL, paths: map[string][]string{"demo": {"demo.js", "missing.js"}}}
	dir := t.TempDir()

	_, err := New().WithProvider(provider).
		WithPackage(library.Package{Name: "demo", Version: "1"}).
		Vendor(context.Background(), dir)
	if err == nil {
		t.Fatal("accepted a failed download")
	}

	if _, err := os.Stat(filepath.Join(dir, LockFile)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("lock written after a failure: %v", err)
	}
}

func TestFromFS(t *testing.T) {
	js := "export default 1"
	css := "body{}"
	lock := Lock{LockfileVersion: 1, Packages: []LockedPackage{{
		Name: "demo", Version: "1.0.0",
		Files: []LockedFile{
			{Path: "demo/demo.js", As: "demo", Integrity: assets.Integrity([]byte(js))},
			{Path: "demo/demo.css", As: "demo", Integrity: assets.Integrity([]byte(css))},
		},
	}}}
	data, _ := json.Marshal(lock)

	fsys := fstest.MapFS{
		"vendor/importmap.lock.json": {Data: data},
		"vendor/demo/demo.js":        {Data: []byte(js)},
		"vendor/demo/demo.css":       {Data: []byte(css)},
		"admin.js":                   {Data: []byte("import 'demo'")},
	}

	im, err := FromFS(fsys, "vendor/importmap.lock.json")
	if err != nil {
		t.Fatal(err)
	}

	im.WithURLFunc(func(file string) string {
		return "/admin/assets/" + file
	}).ModulePreload("demo")

	jsURL := "/admin/assets/vendor/demo/demo.js?v=" + im.manifest.Version("vendor/demo/demo.js")
	if got := im.Structure.Imports["demo"]; got != jsURL {
		t.Fatalf("import = %q, want %q", got, jsURL)
	}

	if got := im.URL("admin.js"); !strings.HasPrefix(got, "/admin/assets/admin.js?v=") {
		t.Errorf("URL of an unlocked file = %q", got)
	}

	plain, _ := im.Render()
	if strings.Contains(string(plain), "integrity") {
		t.Errorf("integrity without WithIntegrity: %s", plain)
	}

	out, err := im.RenderWith(WithIntegrity())
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		`integrity="` + assets.Integrity([]byte(css)) + `" crossorigin="anonymous"/>`,
		`"integrity": {`,
		`"` + strings.ReplaceAll(jsURL, "&", `\u0026`) + `": "` + assets.Integrity([]byte(js)) + `"`,
		`<link rel="modulepreload" href="` + jsURL + `" integrity="` + assets.Integrity([]byte(js)) + `"/>`,
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("render misses %s:\n%s", want, out)
		}
	}

	// ServeFile serves every file of the file system.
	rec := httptest.NewRecorder()
	im.ServeFile(rec, httptest.NewRequest(http.MethodGet, "/admin.js", nil), "admin.js")

	if rec.Code != http.StatusOK || rec.Body.String() != "import 'demo'" {
		t.Errorf("ServeFile = %d %q", rec.Code, rec.Body.String())
	}
}

func TestFromFSRejectsChangedOrMissingFiles(t *testing.T) {
	lock := Lock{LockfileVersion: 1, Packages: []LockedPackage{{
		Name:  "demo",
		Files: []LockedFile{{Path: "demo/demo.js", As: "demo", Integrity: assets.Integrity([]byte("one"))}},
	}}}
	data, _ := json.Marshal(lock)

	changed := fstest.MapFS{LockFile: {Data: data}, "demo/demo.js": {Data: []byte("two")}}
	if _, err := FromFS(changed, LockFile); err == nil || !strings.Contains(err.Error(), "integrity") {
		t.Errorf("changed file: %v", err)
	}

	missing := fstest.MapFS{LockFile: {Data: data}}
	if _, err := FromFS(missing, LockFile); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Errorf("missing file: %v", err)
	}

	if _, err := FromFS(fstest.MapFS{}, LockFile); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing lock: %v", err)
	}
}
