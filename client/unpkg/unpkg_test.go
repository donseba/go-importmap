package unpkg

import (
	"net/http"
	"net/http/httptest"
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/donseba/go-importmap/library"
)

// useDefaults points the package default URLs at a test server for the
// duration of the test, so the zero Client can be tested without the network.
func useDefaults(t *testing.T, apiBaseURL, cdnBaseURL, registryBaseURL string) {
	t.Helper()

	api, cdn, registry := defaultApiBaseURL, defaultCdnBaseURL, defaultRegistryBaseURL
	defaultApiBaseURL, defaultCdnBaseURL, defaultRegistryBaseURL = apiBaseURL, cdnBaseURL, registryBaseURL

	t.Cleanup(func() {
		defaultApiBaseURL, defaultCdnBaseURL, defaultRegistryBaseURL = api, cdn, registry
	})
}

func TestFetchPackageFiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bootstrap@5.3.3/" || !r.URL.Query().Has("meta") {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"files":[{"path":"/dist/js/bootstrap.min.js","type":"text/javascript"}]}`))
	}))
	defer server.Close()
	client := New()
	client.apiBaseURL = server.URL + "/%s@%s/?meta"
	client.cdnBaseURL = server.URL + "/%s@%s/"

	files, version, err := client.FetchPackageFiles(t.Context(), "bootstrap", "5.3.3")
	if err != nil {
		t.Fatal(err)
	}
	if version != "5.3.3" || len(files) != 1 || files[0].LocalPath != "dist/js/bootstrap.min.js" ||
		files[0].Path != server.URL+"/bootstrap@5.3.3/dist/js/bootstrap.min.js" {
		t.Fatalf("unexpected version or files: %q, %#v", version, files)
	}
}

func TestFetchPackageFilesLatestVersionAndDirectories(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path.Clean(r.URL.Path) {
		case "/registry/bootstrap":
			_, _ = w.Write([]byte(`{"dist-tags":{"latest":"5.3.8"}}`))
		case "/bootstrap@5.3.8":
			_, _ = w.Write([]byte(`{"files":[{"path":"/dist","type":"directory"},{"path":"/LICENSE","type":"file"}]}`))
		case "/bootstrap@5.3.8/dist":
			_, _ = w.Write([]byte(`{"files":[{"path":"/dist/css","type":"directory"},{"path":"/dist/js/bootstrap.min.js","type":"file"}]}`))
		case "/bootstrap@5.3.8/dist/css":
			_, _ = w.Write([]byte(`{"files":[{"path":"/dist/css/bootstrap.min.css","type":"file"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	useDefaults(t, server.URL+"/%s@%s/?meta", server.URL+"/%s@%s/", server.URL+"/registry/")

	// The zero Client uses the default URLs, and no version asks the
	// registry for the latest one.
	var client Client

	files, version, err := client.FetchPackageFiles(t.Context(), "bootstrap", "")
	if err != nil {
		t.Fatal(err)
	}

	base := server.URL + "/bootstrap@5.3.8/"
	want := library.Files{
		{Type: library.FileTypeCSS, Path: base + "dist/css/bootstrap.min.css", LocalPath: "dist/css/bootstrap.min.css"},
		{Type: library.FileTypeJS, Path: base + "dist/js/bootstrap.min.js", LocalPath: "dist/js/bootstrap.min.js"},
		{Type: library.FileTypeOther, Path: base + "LICENSE", LocalPath: "LICENSE"},
	}
	if version != "5.3.8" || !slices.Equal(files, want) {
		t.Fatalf("FetchPackageFiles = %q, %+v, want 5.3.8, %+v", version, files, want)
	}
}

func TestFetchPackageFilesSkipsUnreadableDirectories(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if path.Clean(r.URL.Path) != "/bootstrap@5.3.3" {
			http.NotFound(w, r)
			return
		}

		_, _ = w.Write([]byte(`{"files":[{"path":"/dist","type":"directory"},{"path":"/LICENSE","type":"file"}]}`))
	}))
	defer server.Close()

	tests := []struct {
		name string
		cdn  string
	}{
		// The listing of /dist is a 404 page, which is not JSON.
		{"invalid listing", server.URL + "/%s@%s/"},
		// The listing of /dist cannot be requested at all.
		{"unreachable listing", "ftp://cdn.example.invalid/%s@%s/"},
	}

	for _, tt := range tests {
		client := New()
		client.apiBaseURL = server.URL + "/%s@%s/?meta"
		client.cdnBaseURL = tt.cdn

		files, version, err := client.FetchPackageFiles(t.Context(), "bootstrap", "5.3.3")
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}

		want := library.Files{{
			Type:      library.FileTypeOther,
			Path:      strings.Replace(tt.cdn, "%s@%s", "bootstrap@5.3.3", 1) + "LICENSE",
			LocalPath: "LICENSE",
		}}
		if version != "5.3.3" || !slices.Equal(files, want) {
			t.Errorf("%s: FetchPackageFiles = %q, %+v, want 5.3.3, %+v", tt.name, version, files, want)
		}
	}
}

func TestFetchPackageFilesErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/status/bootstrap@5.3.3/":
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		case "/garbage/bootstrap@5.3.3/", "/registry/bootstrap":
			_, _ = w.Write([]byte("<html>"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	tests := []struct {
		name     string
		api      string
		registry string
		version  string
		want     string
	}{
		{"invalid API URL", "http://example.invalid/\x7f%s@%s/?meta", server.URL + "/registry/", "5.3.3", "invalid control character"},
		{"unreachable API", "ftp://example.invalid/%s@%s/?meta", server.URL + "/registry/", "5.3.3", "unsupported protocol scheme"},
		{"API error status", server.URL + "/status/%s@%s/?meta", server.URL + "/registry/", "5.3.3", "unpkg API responded with status 503"},
		{"API invalid JSON", server.URL + "/garbage/%s@%s/?meta", server.URL + "/registry/", "5.3.3", "invalid character '<'"},
		{"unreachable registry", server.URL + "/%s@%s/?meta", "ftp://example.invalid/", "", "unsupported protocol scheme"},
		{"registry invalid JSON", server.URL + "/%s@%s/?meta", server.URL + "/registry/", "", "invalid character '<'"},
	}

	for _, tt := range tests {
		client := New()
		client.apiBaseURL = tt.api
		client.cdnBaseURL = server.URL + "/%s@%s/"
		client.registryBaseURL = tt.registry

		files, version, err := client.FetchPackageFiles(t.Context(), "bootstrap", tt.version)
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: error = %v, want one containing %q", tt.name, err, tt.want)
		}

		if files != nil || version != "" {
			t.Errorf("%s: FetchPackageFiles = %q, %+v with an error, want nothing", tt.name, version, files)
		}
	}
}
