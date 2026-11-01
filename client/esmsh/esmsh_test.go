package esmsh

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/donseba/go-importmap/library"
)

func TestFetchPackageFilesJSONMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bootstrap@5.3.3" || !r.URL.Query().Has("meta") {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"name":"bootstrap","version":"5.3.3","module":"/bootstrap@5.3.3/es2022/bootstrap.mjs"}`))
	}))
	defer server.Close()
	client := New()
	client.apiBaseURL = server.URL + "/"

	files, version, err := client.FetchPackageFiles(t.Context(), "bootstrap", "5.3.3")
	if err != nil {
		t.Fatal(err)
	}
	if version != "5.3.3" || len(files) != 1 || files[0].Path != server.URL+"/bootstrap@5.3.3/es2022/bootstrap.mjs" ||
		files[0].LocalPath != "bootstrap@5.3.3/es2022/bootstrap.mjs" {
		t.Fatalf("unexpected metadata: %q, %#v", version, files)
	}
}

func TestFetchPackageFilesLegacyMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("/* esm.sh - bootstrap@5.3.3 */\nexport * from \"/bootstrap@5.3.3/es2022/bootstrap.mjs\";"))
	}))
	defer server.Close()
	client := New()
	client.apiBaseURL = server.URL + "/"

	files, version, err := client.FetchPackageFiles(t.Context(), "bootstrap", "5.3.3")
	if err != nil {
		t.Fatal(err)
	}
	if version != "5.3.3" || len(files) != 1 || files[0].Path != server.URL+"/bootstrap@5.3.3/es2022/bootstrap.mjs" {
		t.Fatalf("unexpected metadata: %q, %#v", version, files)
	}
}

func TestFetchPackageFilesLatestVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Without a version the package name alone is asked for.
		if r.URL.Path != "/htmx.org" || !r.URL.Query().Has("meta") {
			http.NotFound(w, r)
			return
		}

		_, _ = w.Write([]byte(`{"name":"htmx.org","version":"2.0.4","module":"/htmx.org@2.0.4/es2022/htmx.mjs"}`))
	}))
	defer server.Close()
	client := New()
	client.apiBaseURL = server.URL + "/"

	files, version, err := client.FetchPackageFiles(t.Context(), "htmx.org", "")
	if err != nil {
		t.Fatal(err)
	}

	want := library.Files{{
		Type:      library.FileTypeOther,
		Path:      server.URL + "/htmx.org@2.0.4/es2022/htmx.mjs",
		LocalPath: "htmx.org@2.0.4/es2022/htmx.mjs",
	}}
	if version != "2.0.4" || !slices.Equal(files, want) {
		t.Fatalf("FetchPackageFiles = %q, %+v, want 2.0.4, %+v", version, files, want)
	}
}

func TestFetchPackageFilesErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/status@1.0.0":
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		case "/short@1.0.0":
			w.Header().Set("Content-Length", "1024")
			_, _ = w.Write([]byte("/* esm.sh - short@1.0.0 */"))
		case "/noversion@1.0.0":
			_, _ = w.Write([]byte(`export * from "/noversion@1.0.0/es2022/noversion.mjs";`))
		case "/noexport@1.0.0":
			_, _ = w.Write([]byte("/* esm.sh - noexport@1.0.0 */\nexport * from \"/other@1.0.0/es2022/other.mjs\";"))
		case "/absolute@1.0.0":
			// JSON whose module is not a path on esm.sh is read as the
			// older JavaScript output, which it is not.
			_, _ = w.Write([]byte(`{"version":"1.0.0","module":"https://cdn.example/absolute.mjs"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	tests := []struct {
		name    string
		api     string
		pkgName string
		want    string
	}{
		{"invalid API URL", "http://example.invalid/\x7f/", "htmx.org", "invalid control character"},
		{"unreachable API", "ftp://example.invalid/", "htmx.org", "unsupported protocol scheme"},
		{"error status", server.URL + "/", "status", "esm.sh meta API responded with code 503"},
		{"incomplete body", server.URL + "/", "short", "unexpected EOF"},
		{"no version comment", server.URL + "/", "noversion", "failed to parse version"},
		{"no export of the package", server.URL + "/", "noexport", "failed to parse export file URL"},
		{"module not on esm.sh", server.URL + "/", "absolute", "failed to parse version"},
	}

	for _, tt := range tests {
		client := New()
		client.apiBaseURL = tt.api

		files, version, err := client.FetchPackageFiles(t.Context(), tt.pkgName, "1.0.0")
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: error = %v, want one containing %q", tt.name, err, tt.want)
		}

		if files != nil || version != "" {
			t.Errorf("%s: FetchPackageFiles = %q, %+v with an error, want nothing", tt.name, version, files)
		}
	}
}
