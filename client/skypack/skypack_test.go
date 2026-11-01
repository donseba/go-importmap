package skypack

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/donseba/go-importmap/library"
)

func TestFetchPackageFiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/package/bootstrap":
			_, _ = w.Write([]byte(`{"name":"bootstrap","version":"5.2.0"}`))
		case "/browse/bootstrap/5.1.3":
			_, _ = w.Write([]byte(`{"files":[{"name":"bootstrap.js","url":"https://cdn.skypack.dev/bootstrap@5.1.3/bootstrap.js"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := New()
	client.packageApiBaseURL = server.URL + "/package/"
	client.browseApiBaseURL = server.URL + "/browse/"

	files, version, err := client.FetchPackageFiles(t.Context(), "bootstrap", "5.1.3")
	if err != nil {
		t.Fatal(err)
	}
	if version != "5.1.3" || len(files) != 1 || files[0].LocalPath != "bootstrap.js" ||
		files[0].Path != "https://cdn.skypack.dev/bootstrap@5.1.3/bootstrap.js" {
		t.Fatalf("unexpected version or files: %q, %#v", version, files)
	}
}

func TestFetchPackageFilesLatestVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/package/htmx.org":
			_, _ = w.Write([]byte(`{"name":"htmx.org","version":"2.0.4"}`))
		case "/browse/htmx.org/2.0.4":
			_, _ = w.Write([]byte(`{"files":[
				{"name":"dist/htmx.min.js","url":"https://cdn.skypack.dev/htmx.org@2.0.4/dist/htmx.min.js"},
				{"name":"dist/htmx.min.css","url":"https://cdn.skypack.dev/htmx.org@2.0.4/dist/htmx.min.css"}
			]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := New()
	client.packageApiBaseURL = server.URL + "/package/"
	client.browseApiBaseURL = server.URL + "/browse/"

	files, version, err := client.FetchPackageFiles(t.Context(), "htmx.org", "")
	if err != nil {
		t.Fatal(err)
	}

	want := library.Files{
		{Type: library.FileTypeJS, Path: "https://cdn.skypack.dev/htmx.org@2.0.4/dist/htmx.min.js", LocalPath: "dist/htmx.min.js"},
		{Type: library.FileTypeCSS, Path: "https://cdn.skypack.dev/htmx.org@2.0.4/dist/htmx.min.css", LocalPath: "dist/htmx.min.css"},
	}
	if version != "2.0.4" || !slices.Equal(files, want) {
		t.Fatalf("FetchPackageFiles = %q, %+v, want 2.0.4, %+v", version, files, want)
	}
}

func TestFetchPackageFilesFallsBackToEntryPoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/package/canvas-confetti":
			_, _ = w.Write([]byte(`{"name":"canvas-confetti","version":"1.9.3"}`))
		case "/browse/canvas-confetti/1.9.3":
			_, _ = w.Write([]byte(`{"files":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := New()
	client.packageApiBaseURL = server.URL + "/package/"
	client.browseApiBaseURL = server.URL + "/browse/"
	client.cdnBaseURL = server.URL + "/cdn/"

	files, version, err := client.FetchPackageFiles(t.Context(), "canvas-confetti", "1.9.3")
	if err != nil {
		t.Fatal(err)
	}

	want := library.Files{{
		Type:      library.FileTypeOther,
		Path:      server.URL + "/cdn/canvas-confetti@1.9.3",
		LocalPath: "canvas-confetti",
	}}
	if version != "1.9.3" || !slices.Equal(files, want) {
		t.Fatalf("FetchPackageFiles = %q, %+v, want 1.9.3, %+v", version, files, want)
	}
}

func TestFetchPackageFilesErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/package/status":
			http.Error(w, "unavailable", http.StatusBadGateway)
		case "/package/garbage", "/browse/broken/1.0.0":
			_, _ = w.Write([]byte("<html>"))
		case "/package/htmx.org", "/package/missing", "/package/broken":
			_, _ = w.Write([]byte(`{"version":"1.0.0"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	tests := []struct {
		name    string
		pkgAPI  string
		browse  string
		pkgName string
		want    string
	}{
		{"invalid package URL", "http://example.invalid/\x7f/", server.URL + "/browse/", "htmx.org", "invalid control character"},
		{"unreachable package API", "ftp://example.invalid/", server.URL + "/browse/", "htmx.org", "unsupported protocol scheme"},
		{"package error status", server.URL + "/package/", server.URL + "/browse/", "status", "Skypack package API responded with code 502"},
		{"package invalid JSON", server.URL + "/package/", server.URL + "/browse/", "garbage", "invalid character '<'"},
		{"invalid browse URL", server.URL + "/package/", "http://example.invalid/\x7f/", "htmx.org", "invalid control character"},
		{"unreachable browse API", server.URL + "/package/", "ftp://example.invalid/", "htmx.org", "unsupported protocol scheme"},
		{"browse error status", server.URL + "/package/", server.URL + "/browse/", "missing", "skypack browse API responded with code 404"},
		{"browse invalid JSON", server.URL + "/package/", server.URL + "/browse/", "broken", "invalid character '<'"},
	}

	for _, tt := range tests {
		client := New()
		client.packageApiBaseURL = tt.pkgAPI
		client.browseApiBaseURL = tt.browse

		files, version, err := client.FetchPackageFiles(t.Context(), tt.pkgName, "")
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: error = %v, want one containing %q", tt.name, err, tt.want)
		}

		if files != nil || version != "" {
			t.Errorf("%s: FetchPackageFiles = %q, %+v with an error, want nothing", tt.name, version, files)
		}
	}
}
