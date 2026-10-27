package assets

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// writeFile writes content to dir/name with the modification time at.
func writeFile(t *testing.T, dir, name, content string, at time.Time) {
	t.Helper()

	filename := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.Chtimes(filename, at, at); err != nil {
		t.Fatal(err)
	}
}

// get serves target, a request path, from m.
func get(m *Manifest, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	m.ServeFile(rec, req, req.URL.Path)

	return rec
}

func TestReload(t *testing.T) {
	dir := t.TempDir()
	start := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	writeFile(t, dir, "site.css", "body{}", start)

	m, err := Fingerprint(os.DirFS(dir), Reload())
	if err != nil {
		t.Fatal(err)
	}

	if !m.Reloads() {
		t.Fatal("Reloads is false")
	}

	before := m.Version("site.css")

	writeFile(t, dir, "site.css", "body{color:red}", start.Add(time.Second))
	writeFile(t, dir, "app/new.js", "export {}", start)

	if m.Version("site.css") == before || m.Version("site.css") == "" {
		t.Errorf("a changed file kept its version %q", before)
	}

	if got := m.Integrity("site.css"); got != Integrity([]byte("body{color:red}")) {
		t.Errorf("integrity = %q", got)
	}

	if !m.Has("app/new.js") {
		t.Error("an added file is unknown")
	}

	if err := os.Remove(filepath.Join(dir, "app", "new.js")); err != nil {
		t.Fatal(err)
	}

	if m.Has("app/new.js") {
		t.Error("a removed file is known")
	}

	rec := get(m, "/site.css?v="+m.Version("site.css"))
	if rec.Body.String() != "body{color:red}" || rec.Header().Get("Cache-Control") != immutable {
		t.Errorf("served %q with %q", rec.Body.String(), rec.Header().Get("Cache-Control"))
	}
}

func TestReloadIsSafeForConcurrentUse(t *testing.T) {
	dir := t.TempDir()
	start := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	writeFile(t, dir, "a.js", "1", start)

	m := MustFingerprint(os.DirFS(dir), Reload())

	var wg sync.WaitGroup

	for i := range 8 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for range 20 {
				_ = m.URL("a.js")
				_ = m.Files()
				_ = get(m, "/a.js")
			}
		}()

		if i%2 == 0 {
			writeFile(t, dir, "a.js", string(rune('a'+i)), start.Add(time.Duration(i)*time.Second))
		}
	}

	wg.Wait()

	writeFile(t, dir, "a.js", "final", start.Add(time.Hour))

	if got := m.Integrity("a.js"); got != Integrity([]byte("final")) {
		t.Errorf("integrity = %q", got)
	}
}

func TestWithoutReloadFilesStayFingerprinted(t *testing.T) {
	dir := t.TempDir()
	start := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	writeFile(t, dir, "site.css", "body{}", start)

	m := MustFingerprint(os.DirFS(dir))
	version := m.Version("site.css")

	writeFile(t, dir, "site.css", "body{color:red}", start.Add(time.Second))

	if m.Reloads() || m.Version("site.css") != version {
		t.Error("a manifest without Reload changed")
	}
}

// TestServeFileChangedOnDisk checks that a file changed since it was
// fingerprinted is never labelled with the version it no longer has.
func TestServeFileChangedOnDisk(t *testing.T) {
	dir := t.TempDir()
	start := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	writeFile(t, dir, "site.css", "body{}", start)

	m := MustFingerprint(os.DirFS(dir))
	old := m.Version("site.css")

	if rec := get(m, "/site.css?v="+old); rec.Header().Get("ETag") != `"`+old+`"` ||
		rec.Header().Get("Cache-Control") != immutable {
		t.Fatalf("unchanged file: %v", rec.Header())
	}

	writeFile(t, dir, "site.css", "body{color:red}", start.Add(time.Second))

	rec := get(m, "/site.css?v="+old)
	if rec.Body.String() != "body{color:red}" {
		t.Fatalf("body = %q", rec.Body.String())
	}

	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("new content cached as the old version: %q", got)
	}

	current := MustFingerprint(os.DirFS(dir)).Version("site.css")
	if got := rec.Header().Get("ETag"); got != `"`+current+`"` {
		t.Errorf("ETag = %s, want the new content's %q (old %q)", got, current, old)
	}

	// The same size with another time counts as changed too.
	writeFile(t, dir, "site.css", "body{color:blu}", start.Add(2*time.Second))

	rec = get(m, "/site.css?v="+old)
	if rec.Header().Get("ETag") == `"`+old+`"` || rec.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("same size, new time: %v", rec.Header())
	}

	if rec := get(m, "/site.css?v="+old); rec.Code != http.StatusOK {
		t.Errorf("status %d", rec.Code)
	}
}
