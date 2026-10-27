package assets

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func fixture(t *testing.T) *Manifest {
	t.Helper()

	m, err := Fingerprint(fstest.MapFS{
		"htmx/htmx.esm.min.js": {Data: []byte("export default {}")},
		"site.css":             {Data: []byte("body{}")},
		".hidden":              {Data: []byte("secret")},
		".git/config":          {Data: []byte("secret")},
		"htmx/.importmap-123":  {Data: []byte("partial")},
	})
	if err != nil {
		t.Fatal(err)
	}

	return m
}

func TestFingerprint(t *testing.T) {
	m := fixture(t)

	if got := strings.Join(m.Files(), ","); got != "htmx/htmx.esm.min.js,site.css" {
		t.Fatalf("files = %s", got)
	}

	version := m.Version("htmx/htmx.esm.min.js")
	if len(version) != 16 || version == m.Version("site.css") {
		t.Fatalf("version = %q", version)
	}

	if got := m.Integrity("/site.css"); got != Integrity([]byte("body{}")) || !strings.HasPrefix(got, "sha384-") {
		t.Fatalf("integrity = %q", got)
	}

	if m.Version("missing.js") != "" || m.Integrity("missing.js") != "" || m.Has("missing.js") {
		t.Fatal("unknown file has a fingerprint")
	}

	if got := m.URL("site.css"); got != "/site.css?v="+m.Version("site.css") {
		t.Fatalf("URL = %q", got)
	}

	m.WithURLFunc(func(file string) string {
		return "/admin/assets/" + file + "?x=1#top"
	})

	if got := m.URL("site.css"); got != "/admin/assets/site.css?x=1&v="+m.Version("site.css")+"#top" {
		t.Fatalf("routed URL = %q", got)
	}

	if got := m.URL("missing.js"); got != "/admin/assets/missing.js?x=1#top" {
		t.Fatalf("unknown file URL = %q", got)
	}
}

func TestServeFile(t *testing.T) {
	m := fixture(t)
	version := m.Version("site.css")

	serve := func(method, target string, header http.Header) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, nil)
		for key, values := range header {
			req.Header[key] = values
		}

		rec := httptest.NewRecorder()
		m.ServeFile(rec, req, req.URL.Path)

		return rec
	}

	rec := serve(http.MethodGet, "/site.css?v="+version, nil)
	if rec.Code != http.StatusOK || rec.Body.String() != "body{}" {
		t.Fatalf("versioned = %d %q", rec.Code, rec.Body.String())
	}

	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Errorf("versioned Cache-Control = %q", got)
	}

	if got := rec.Header().Get("ETag"); got != `"`+version+`"` {
		t.Errorf("ETag = %q", got)
	}

	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/css") {
		t.Errorf("Content-Type = %q", got)
	}

	for _, target := range []string{"/site.css", "/site.css?v=stale"} {
		if got := serve(http.MethodGet, target, nil).Header().Get("Cache-Control"); got != "no-cache" {
			t.Errorf("%s Cache-Control = %q", target, got)
		}
	}

	revalidate := http.Header{"If-None-Match": {`"` + version + `"`}}
	if rec := serve(http.MethodGet, "/site.css", revalidate); rec.Code != http.StatusNotModified {
		t.Errorf("revalidation = %d", rec.Code)
	}

	module := serve(http.MethodGet, "/htmx/htmx.esm.min.js", nil)
	if !strings.HasPrefix(module.Header().Get("Content-Type"), "text/javascript") {
		t.Errorf("module Content-Type = %q", module.Header().Get("Content-Type"))
	}

	for _, target := range []string{"/missing.js", "/.hidden", "/htmx/", "/htmx", "/../site.css"} {
		if rec := serve(http.MethodGet, target, nil); rec.Code != http.StatusNotFound {
			t.Errorf("%s = %d", target, rec.Code)
		}
	}

	if rec := serve(http.MethodPost, "/site.css", nil); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST = %d", rec.Code)
	}

	if rec := serve(http.MethodHead, "/site.css", nil); rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Errorf("HEAD = %d %q", rec.Code, rec.Body.String())
	}
}

func TestVersioned(t *testing.T) {
	cases := map[string]string{
		"/a.js":          "/a.js?v=1",
		"/a.js?x=y":      "/a.js?x=y&v=1",
		"/a.js#frag":     "/a.js?v=1#frag",
		"/a.js?x#frag":   "/a.js?x&v=1#frag",
		"https://c/a.js": "https://c/a.js?v=1",
	}

	for in, want := range cases {
		if got := Versioned(in, "1"); got != want {
			t.Errorf("Versioned(%q) = %q, want %q", in, got, want)
		}
	}

	if got := Versioned("/a.js", ""); got != "/a.js" {
		t.Errorf("empty version = %q", got)
	}
}

func TestFingerprintFails(t *testing.T) {
	missing := os.DirFS(filepath.Join(t.TempDir(), "missing"))

	if _, err := Fingerprint(missing); err == nil {
		t.Fatal("fingerprinted a missing directory")
	}

	defer func() {
		if recover() == nil {
			t.Fatal("MustFingerprint did not panic")
		}
	}()

	MustFingerprint(missing)
}
