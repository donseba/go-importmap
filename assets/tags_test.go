package assets

import (
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func sectionFS() fstest.MapFS {
	return fstest.MapFS{
		"sections/pages.css":     {Data: []byte("body{}")},
		"sections/page.gohtml":   {Data: []byte("{{.}}")},
		"sections/.hidden.css":   {Data: []byte("a{}")},
		"other/unrelated.js":     {Data: []byte("x")},
		"sections/sub/extra.css": {Data: []byte("b{}")},
	}
}

func TestOnlyFingerprintsTheNamedFiles(t *testing.T) {
	m, err := Fingerprint(sectionFS(), Only("/sections/pages.css"), Only("sections/.hidden.css"))
	if err != nil {
		t.Fatal(err)
	}

	if files := m.Files(); len(files) != 2 || files[0] != "sections/.hidden.css" || files[1] != "sections/pages.css" {
		t.Errorf("Files = %v, want the two named files", files)
	}

	if m.Integrity("sections/pages.css") != Integrity([]byte("body{}")) {
		t.Error("the named file has no integrity")
	}

	rec := httptest.NewRecorder()
	m.ServeFile(rec, httptest.NewRequest(http.MethodGet, "/", nil), "sections/page.gohtml")

	if rec.Code != http.StatusNotFound {
		t.Errorf("a file outside Only is served: %d", rec.Code)
	}
}

func TestOnlyFailsForMissingFiles(t *testing.T) {
	if _, err := Fingerprint(sectionFS(), Only("sections/missing.css")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Fingerprint of a missing file = %v, want fs.ErrNotExist", err)
	}

	if _, err := Fingerprint(sectionFS(), Only("sections")); err == nil {
		t.Error("Fingerprint of a directory did not fail")
	}

	if _, err := Fingerprint(sectionFS(), Only("../x.css")); err == nil {
		t.Error("Fingerprint of an invalid name did not fail")
	}

	// Only without names fingerprints nothing.
	m, err := Fingerprint(sectionFS(), Only())
	if err != nil || len(m.Files()) != 0 {
		t.Errorf("Only() = %v, %v", m.Files(), err)
	}
}

func TestOnlyReloads(t *testing.T) {
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

	write("site.css", "a{}", start)
	write("page.gohtml", "x", start)

	m, err := Fingerprint(os.DirFS(dir), Only("site.css"), Reload())
	if err != nil {
		t.Fatal(err)
	}

	before := m.Version("site.css")

	write("site.css", "a{color:red}", start.Add(time.Second))
	write("added.css", "b{}", start)

	if m.Version("site.css") == before {
		t.Error("the changed file kept its version")
	}

	if m.Has("added.css") || m.Has("page.gohtml") {
		t.Errorf("Reload added files outside Only: %v", m.Files())
	}
}

func TestManifestTagsWithoutIntegrityWhileReloading(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.js"), []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}

	fixed := MustFingerprint(os.DirFS(dir))
	if fixed.TagIntegrity("app.js") != Integrity([]byte("1")) {
		t.Errorf("TagIntegrity of a fixed manifest = %q", fixed.TagIntegrity("app.js"))
	}

	m := MustFingerprint(os.DirFS(dir), Reload())
	if m.TagIntegrity("app.js") != "" || m.Integrity("app.js") == "" {
		t.Errorf("reloading: TagIntegrity = %q, Integrity = %q", m.TagIntegrity("app.js"), m.Integrity("app.js"))
	}

	style, err := m.Stylesheet("app.js")
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(style), "integrity") {
		t.Errorf("Stylesheet of a reloading manifest = %s", style)
	}

	if _, err := m.Stylesheet("missing.css"); err == nil {
		t.Error("Stylesheet of an unknown file did not fail while reloading")
	}
}

func TestManifestStylesheetAndScriptTag(t *testing.T) {
	m := MustFingerprint(fstest.MapFS{
		"app.js":   {Data: []byte("1")},
		"site.css": {Data: []byte("a{}")},
	}).WithURLFunc(func(file string) string { return "/static/" + file })

	script, err := ScriptTag(m.URL("app.js"), m.TagIntegrity("app.js"), "type", "module", "crossorigin", "use-credentials")
	if err != nil {
		t.Fatal(err)
	}

	want := `<script src="/static/app.js?v=` + m.Version("app.js") +
		`" integrity="` + m.Integrity("app.js") +
		`" type="module" crossorigin="use-credentials"></script>`
	if string(script) != want {
		t.Errorf("ScriptTag =\n%s\nwant\n%s", script, want)
	}

	style, err := m.Stylesheet("site.css")
	if err != nil {
		t.Fatal(err)
	}

	want = `<link rel="stylesheet" href="/static/site.css?v=` + m.Version("site.css") +
		`" integrity="` + m.Integrity("site.css") +
		`" crossorigin="anonymous">`
	if string(style) != want {
		t.Errorf("Stylesheet =\n%s\nwant\n%s", style, want)
	}

	if _, err := m.Stylesheet("missing.css"); err == nil {
		t.Error("Stylesheet of an unknown file did not fail")
	}

	if _, err := m.Stylesheet("site.css", "href", "/x.css"); err == nil {
		t.Error("href among the attributes did not fail")
	}

	if _, err := m.Stylesheet("site.css", `on"load`, "x"); err == nil {
		t.Error("an invalid attribute name did not fail")
	}

	if _, err := ScriptTag(m.URL("app.js"), "", "integrity", "sha384-x"); err == nil {
		t.Error("integrity among the attributes did not fail")
	}
}

func TestTagsWithoutIntegrity(t *testing.T) {
	script, err := ScriptTag("/x.js", "", "async", "")
	if err != nil || string(script) != `<script src="/x.js" async></script>` {
		t.Errorf("ScriptTag = %s, %v", script, err)
	}

	style, err := StylesheetTag(`/x.css?a=1&b=2`, "")
	if err != nil || string(style) != `<link rel="stylesheet" href="/x.css?a=1&amp;b=2">` {
		t.Errorf("StylesheetTag = %s, %v", style, err)
	}
}
