package jsdelivr

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
		case "/bootstrap":
			_, _ = w.Write([]byte(`{"tags":{"latest":"5.3.4"},"versions":["5.3.3","5.3.4"]}`))
		case "/bootstrap@5.3.3":
			_, _ = w.Write([]byte(`{"name":"bootstrap","version":"5.3.3","files":[{"type":"directory","name":"dist","files":[{"type":"file","name":"bootstrap.min.js"}]}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := New()
	client.apiBaseURL = server.URL + "/"

	files, version, err := client.FetchPackageFiles(t.Context(), "bootstrap", "5.3.3")
	if err != nil {
		t.Fatal(err)
	}
	if version != "5.3.3" || len(files) != 1 || files[0].LocalPath != "dist/bootstrap.min.js" {
		t.Fatalf("unexpected version or files: %q, %#v", version, files)
	}
}

func TestIncludeMinified(t *testing.T) {
	cdn := New()

	f, _, err := cdn.FetchPackageFiles(t.Context(), "@hotwired/turbo", "8.0.13")

	if err != nil {
		t.Error(err)
		return
	}

	if len(f) != 4 {
		t.Error("files count mismatch")
	}

	t.Log(f)
}

// useDefaults points the package default URLs at a test server for the
// duration of the test, so the zero Client can be tested without the network.
func useDefaults(t *testing.T, apiBaseURL, cdnBaseURL string) {
	t.Helper()

	api, cdn := defaultApiBaseURL, defaultCdnBaseURL
	defaultApiBaseURL, defaultCdnBaseURL = apiBaseURL, cdnBaseURL

	t.Cleanup(func() {
		defaultApiBaseURL, defaultCdnBaseURL = api, cdn
	})
}

// htmxServer serves the jsDelivr API for htmx.org, whose default file is in
// dist, and for alpinejs, whose default file is not.
func htmxServer(t *testing.T) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/htmx.org":
			_, _ = w.Write([]byte(`{"tags":{"latest":"2.0.4"},"versions":["2.0.4","2.0.3"]}`))
		case "/api/htmx.org@2.0.4":
			_, _ = w.Write([]byte(`{"default":"/dist/htmx.min.js","files":[
				{"type":"directory","name":"dist","files":[
					{"type":"file","name":"htmx.esm.js"},
					{"type":"file","name":"htmx.min.css"},
					{"type":"directory","name":"ext","files":[{"type":"file","name":"json-enc.js"}]},
					{"type":"file","name":"README.md"}
				]},
				{"type":"directory","name":"src","files":[{"type":"file","name":"htmx.js"}]},
				{"type":"file","name":"LICENSE"}
			]}`))
		case "/api/alpinejs":
			_, _ = w.Write([]byte(`{"tags":{"latest":"3.14.9"},"versions":["3.14.9"]}`))
		case "/api/alpinejs@3.14.9":
			_, _ = w.Write([]byte(`{"default":"/index.js","files":[
				{"type":"directory","name":"src","files":[{"type":"file","name":"alpine.css"}]},
				{"type":"file","name":"index.min.js"}
			]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	return server
}

func TestFetchPackageFilesDistAndMinified(t *testing.T) {
	server := htmxServer(t)
	useDefaults(t, server.URL+"/api/", server.URL+"/cdn/")

	// The zero Client uses the default URLs, and no version means the
	// latest one.
	var client Client

	files, version, err := client.FetchPackageFiles(t.Context(), "htmx.org", "")
	if err != nil {
		t.Fatal(err)
	}

	// Only dist is listed, as the default file is in it, and every JS or
	// CSS file that is not minified gets its minified twin.
	base := server.URL + "/cdn/htmx.org@2.0.4/"
	want := library.Files{
		{Type: library.FileTypeJS, Path: base + "dist/htmx.esm.js", LocalPath: "dist/htmx.esm.js"},
		{Type: library.FileTypeJS, Path: base + "dist/htmx.esm.min.js", LocalPath: "dist/htmx.esm.min.js"},
		{Type: library.FileTypeCSS, Path: base + "dist/htmx.min.css", LocalPath: "dist/htmx.min.css"},
		{Type: library.FileTypeJS, Path: base + "dist/ext/json-enc.js", LocalPath: "dist/ext/json-enc.js"},
		{Type: library.FileTypeJS, Path: base + "dist/ext/json-enc.min.js", LocalPath: "dist/ext/json-enc.min.js"},
		{Type: library.FileTypeOther, Path: base + "dist/README.md", LocalPath: "dist/README.md"},
	}
	if version != "2.0.4" || !slices.Equal(files, want) {
		t.Fatalf("FetchPackageFiles = %q, %+v, want 2.0.4, %+v", version, files, want)
	}
}

func TestFetchPackageFilesWithoutDist(t *testing.T) {
	server := htmxServer(t)
	client := New()
	client.apiBaseURL = server.URL + "/api/"
	client.cdnBaseURL = server.URL + "/cdn/"

	files, version, err := client.FetchPackageFiles(t.Context(), "alpinejs", "3.14.9")
	if err != nil {
		t.Fatal(err)
	}

	base := server.URL + "/cdn/alpinejs@3.14.9/"
	want := library.Files{
		{Type: library.FileTypeCSS, Path: base + "src/alpine.css", LocalPath: "src/alpine.css"},
		{Type: library.FileTypeCSS, Path: base + "src/alpine.min.css", LocalPath: "src/alpine.min.css"},
		{Type: library.FileTypeJS, Path: base + "index.min.js", LocalPath: "index.min.js"},
	}
	if version != "3.14.9" || !slices.Equal(files, want) {
		t.Fatalf("FetchPackageFiles = %q, %+v, want 3.14.9, %+v", version, files, want)
	}
}

func TestFetchPackageFilesESM(t *testing.T) {
	server := htmxServer(t)

	client := NewESM()
	client.apiBaseURL = server.URL + "/api/"
	client.cdnBaseURL = server.URL + "/cdn/"

	files, version, err := client.FetchPackageFiles(t.Context(), "htmx.org", "2.0.4")
	if err != nil {
		t.Fatal(err)
	}

	// The JS files make way for the ESM bundle; the other files stay.
	base := server.URL + "/cdn/htmx.org@2.0.4/"
	want := library.Files{
		{Type: library.FileTypeJS, Path: base + "+esm", LocalPath: "esm-bundle.js"},
		{Type: library.FileTypeCSS, Path: base + "dist/htmx.min.css", LocalPath: "dist/htmx.min.css"},
		{Type: library.FileTypeOther, Path: base + "dist/README.md", LocalPath: "dist/README.md"},
	}
	if version != "2.0.4" || !slices.Equal(files, want) {
		t.Fatalf("ESM FetchPackageFiles = %q, %+v, want 2.0.4, %+v", version, files, want)
	}

	if got := client.SetESM(false); got != client {
		t.Fatal("SetESM returned another client")
	}

	files, _, err = client.FetchPackageFiles(t.Context(), "htmx.org", "2.0.4")
	if err != nil {
		t.Fatal(err)
	}

	if len(files) != 6 || files[0].LocalPath != "dist/htmx.esm.js" {
		t.Fatalf("FetchPackageFiles after SetESM(false) = %+v, want the plain files", files)
	}
}

func TestFetchPackageFilesErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/status":
			http.Error(w, "unavailable", http.StatusInternalServerError)
		case "/garbage", "/broken@1.0.0":
			_, _ = w.Write([]byte("<html>"))
		case "/badversion":
			_, _ = w.Write([]byte(`{"tags":{"latest":"1.0.0\u007f"}}`))
		case "/redirect", "/missing", "/broken":
			_, _ = w.Write([]byte(`{"tags":{"latest":"1.0.0"},"versions":["1.0.0"]}`))
		case "/redirect@1.0.0":
			http.Redirect(w, r, "ftp://example.invalid/redirect@1.0.0", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	tests := []struct {
		name string
		api  string
		pkg  string
		want string
	}{
		{"invalid API URL", "http://example.invalid/\x7f/", "htmx.org", "invalid control character"},
		{"unreachable API", "ftp://example.invalid/", "htmx.org", "unsupported protocol scheme"},
		{"package error status", server.URL + "/", "status", "client api responded with code 500"},
		{"package invalid JSON", server.URL + "/", "garbage", "invalid character '<'"},
		{"invalid version URL", server.URL + "/", "badversion", "invalid control character"},
		{"unreachable version", server.URL + "/", "redirect", "unsupported protocol scheme"},
		{"version error status", server.URL + "/", "missing", "client api responded with code 404"},
		{"version invalid JSON", server.URL + "/", "broken", "invalid character '<'"},
	}

	for _, tt := range tests {
		client := New()
		client.apiBaseURL = tt.api

		files, version, err := client.FetchPackageFiles(t.Context(), tt.pkg, "")
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: error = %v, want one containing %q", tt.name, err, tt.want)
		}

		if files != nil || version != "" {
			t.Errorf("%s: FetchPackageFiles = %q, %+v with an error, want nothing", tt.name, version, files)
		}
	}
}
