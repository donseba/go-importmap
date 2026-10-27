package importmap

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/donseba/go-importmap/assets"
)

// localFS is a file system with a vendored package and the application's
// own scripts.
func localFS(t *testing.T) fstest.MapFS {
	t.Helper()

	js := "export default 1"
	lock := Lock{LockfileVersion: 1, Packages: []LockedPackage{{
		Name:  "demo",
		Files: []LockedFile{{Path: "demo/demo.js", As: "demo", Integrity: assets.Integrity([]byte(js))}},
	}}}
	data, _ := json.Marshal(lock)

	return fstest.MapFS{
		"vendor/importmap.lock.json": {Data: data},
		"vendor/demo/demo.js":        {Data: []byte(js)},
		"editor/editor.js":           {Data: []byte("import './bundle.js'")},
		"editor/bundle.js":           {Data: []byte("export const bundle = 1")},
		"icons.svg":                  {Data: []byte("<svg/>")},
		"vendor/demo/LICENSE":        {Data: []byte("MIT")},
	}
}

func TestManifestAndServeFile(t *testing.T) {
	im, err := FromFS(localFS(t), "vendor/importmap.lock.json")
	if err != nil {
		t.Fatal(err)
	}

	manifest := im.Manifest()
	if manifest == nil || !manifest.Has("editor/bundle.js") || !manifest.Has("vendor/demo/demo.js") {
		t.Fatal("Manifest misses the file system's files")
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x?v="+manifest.Version("icons.svg"), nil)
	im.ServeFile(rec, req, "icons.svg")

	if rec.Code != http.StatusOK || rec.Body.String() != "<svg/>" ||
		rec.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Errorf("ServeFile = %d %q %v", rec.Code, rec.Body.String(), rec.Header())
	}

	empty := New()
	if empty.Manifest() != nil {
		t.Error("a map without files has a manifest")
	}

	rec = httptest.NewRecorder()
	empty.ServeFile(rec, httptest.NewRequest(http.MethodGet, "/icons.svg", nil), "icons.svg")

	if rec.Code != http.StatusNotFound {
		t.Errorf("ServeFile without files = %d", rec.Code)
	}
}

func TestWithVersionedPaths(t *testing.T) {
	im, err := FromFS(localFS(t), "vendor/importmap.lock.json")
	if err != nil {
		t.Fatal(err)
	}

	im.WithVersionedPaths("editor/bundle.js", "/icons.svg", "missing.js")
	im.WithURLFunc(func(file string) string { return "/admin/assets/" + file })

	manifest := im.Manifest()
	bundle := "/admin/assets/editor/bundle.js?v=" + manifest.Version("editor/bundle.js")

	out, err := im.RenderWith(WithIntegrity())
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		`"/admin/assets/editor/bundle.js": "` + strings.ReplaceAll(bundle, "&", `\u0026`) + `"`,
		`"/admin/assets/icons.svg": "/admin/assets/icons.svg?v=` + manifest.Version("icons.svg") + `"`,
		`"` + bundle + `": "` + manifest.Integrity("editor/bundle.js") + `"`,
		`"demo": "/admin/assets/vendor/demo/demo.js?v=`,
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("render misses %s:\n%s", want, out)
		}
	}

	if strings.Contains(string(out), "missing.js") || strings.Contains(string(out), "editor/editor.js") {
		t.Errorf("render maps files it was not given:\n%s", out)
	}

	// Structure holds what was configured; the entries are derived.
	if _, ok := im.Structure.Imports["/admin/assets/editor/bundle.js"]; ok {
		t.Error("versioned paths written into Structure")
	}

	data, err := im.Marshal()
	if err != nil || !strings.Contains(string(data), `"/admin/assets/editor/bundle.js"`) {
		t.Errorf("Marshal misses versioned paths: %s %v", data, err)
	}

	// A name mapped by hand wins.
	im.WithImport("/admin/assets/icons.svg", "/elsewhere.svg", "")

	out, _ = im.Render()
	if !strings.Contains(string(out), `"/admin/assets/icons.svg": "/elsewhere.svg"`) {
		t.Errorf("WithImport lost to a versioned path:\n%s", out)
	}
}

func TestWithVersionedPathsAll(t *testing.T) {
	im, err := FromFS(localFS(t), "vendor/importmap.lock.json")
	if err != nil {
		t.Fatal(err)
	}

	out, err := im.WithVersionedPaths().Render()
	if err != nil {
		t.Fatal(err)
	}

	for _, file := range im.Manifest().Files() {
		want := `"/assets/` + file + `": "/assets/` + file + `?v=` + im.Manifest().Version(file) + `"`

		// No script loads the lock file or a licence.
		if file == "vendor/importmap.lock.json" || file == "vendor/demo/LICENSE" {
			if strings.Contains(string(out), `"/assets/`+file+`"`) {
				t.Errorf("render maps %s:\n%s", file, out)
			}

			continue
		}

		if !strings.Contains(string(out), want) {
			t.Errorf("render misses %s:\n%s", want, out)
		}
	}

	if out, _ := New().WithVersionedPaths().Render(); out != "" {
		t.Errorf("versioned paths without a manifest: %s", out)
	}
}

func TestImportMapFollowsReloadingAssets(t *testing.T) {
	dir := t.TempDir()
	start := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

	write := func(name, content string, at time.Time) {
		filename := filepath.Join(dir, name)
		if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		if err := os.Chtimes(filename, at, at); err != nil {
			t.Fatal(err)
		}
	}

	write("app.js", "1", start)
	write("site.css", "body{}", start)

	manifest, err := assets.Fingerprint(os.DirFS(dir), assets.Reload())
	if err != nil {
		t.Fatal(err)
	}

	im := New().WithAssets(manifest).WithFile("app", "app.js").WithFile("site", "site.css").WithVersionedPaths()

	before, _ := im.Render()

	write("app.js", "22", start.Add(time.Second))
	write("site.css", "body{color:red}", start.Add(time.Second))
	write("added.js", "3", start)

	after, err := im.RenderWith(WithIntegrity())
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		`"app": "/assets/app.js?v=` + manifest.Version("app.js") + `"`,
		`href="/assets/site.css?v=` + manifest.Version("site.css") +
			`" integrity="` + assets.Integrity([]byte("body{color:red}")) + `"`,
		`"/assets/added.js": "/assets/added.js?v=` + manifest.Version("added.js") + `"`,
		`": "` + assets.Integrity([]byte("22")) + `"`,
	} {
		if !strings.Contains(string(after), want) {
			t.Errorf("render misses %s:\n%s", want, after)
		}
	}

	if before == after {
		t.Error("the import map did not change")
	}
}

func TestWithVersionedPathsPatterns(t *testing.T) {
	cases := map[string]struct {
		patterns []string
		want     []string
	}{
		"globs": {
			patterns: []string{"**/*.js", "*.svg"},
			want:     []string{"editor/bundle.js", "editor/editor.js", "icons.svg", "vendor/demo/demo.js"},
		},
		"exclusions only": {
			patterns: []string{"!vendor/**"},
			want:     []string{"editor/bundle.js", "editor/editor.js", "icons.svg"},
		},
		"globs and exclusions": {
			patterns: []string{"**/*.js", "!editor/b*"},
			want:     []string{"editor/editor.js", "vendor/demo/demo.js"},
		},
		"a licence by name": {
			patterns: []string{"vendor/demo/LICENSE"},
			want:     []string{"vendor/demo/LICENSE"},
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			im, err := FromFS(localFS(t), "vendor/importmap.lock.json")
			if err != nil {
				t.Fatal(err)
			}

			got := strings.Join(im.WithVersionedPaths(c.patterns...).versionedFiles(), ",")
			if got != strings.Join(c.want, ",") {
				t.Errorf("versioned files = %s, want %s", got, strings.Join(c.want, ","))
			}
		})
	}
}

func TestMatchPath(t *testing.T) {
	for _, c := range []struct {
		pattern, file string
		want          bool
	}{
		{"*.js", "app.js", true},
		{"*.js", "editor/app.js", false},
		{"**/*.js", "app.js", true},
		{"**/*.js", "a/b/app.js", true},
		{"vendor/**", "vendor/x/y.js", true},
		{"vendor/**", "vendors/y.js", false},
		{"a/**/z.js", "a/z.js", true},
		{"a/**/z.js", "a/b/c/z.js", true},
		{"[", "[", true},
		{"[", "x", false},
	} {
		if got := matchPath(c.pattern, c.file); got != c.want {
			t.Errorf("matchPath(%q, %q) = %v, want %v", c.pattern, c.file, got, c.want)
		}
	}
}

func TestWithStableIntegrity(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.js"), []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "site.css"), []byte("a{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	render := func(t *testing.T, manifest *assets.Manifest) string {
		t.Helper()

		im := New().
			WithAssets(manifest).
			WithFile("app", "app.js").
			WithFile("site", "site.css").
			ModulePreload("app").
			WithRenderOptions(WithStableIntegrity())

		head, err := im.Render()
		if err != nil {
			t.Fatal(err)
		}

		script, err := im.Script("app.js")
		if err != nil {
			t.Fatal(err)
		}

		style, err := im.Stylesheet("site.css")
		if err != nil {
			t.Fatal(err)
		}

		return string(head) + string(script) + string(style)
	}

	fixed := render(t, assets.MustFingerprint(os.DirFS(dir)))
	if strings.Count(fixed, assets.Integrity([]byte("1"))) != 3 || strings.Count(fixed, assets.Integrity([]byte("a{}"))) != 2 {
		t.Errorf("a fixed manifest renders without integrity:\n%s", fixed)
	}

	reloading := render(t, assets.MustFingerprint(os.DirFS(dir), assets.Reload()))
	if strings.Contains(reloading, "integrity") {
		t.Errorf("a reloading manifest renders integrity:\n%s", reloading)
	}

	// The option holds for the whole render, also next to WithIntegrity.
	im := New().WithAssets(assets.MustFingerprint(os.DirFS(dir), assets.Reload())).WithFile("app", "app.js")

	head, err := im.RenderWith(WithStableIntegrity(), WithIntegrity())
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(head), "integrity") {
		t.Errorf("WithIntegrity overrides WithStableIntegrity:\n%s", head)
	}
}
