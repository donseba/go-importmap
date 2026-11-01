package cdnjs

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/donseba/go-importmap/library"
)

func TestFetchPackageFilesUsesRequestedVersion(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/libraries/htmx":
			_, _ = w.Write([]byte(`{"version":"4.0.0","filename":"htmx.min.js","assets":[{"version":"4.0.0","files":["htmx.min.js"]}]}`))
		case "/libraries/htmx/4.0.0":
			_, _ = w.Write([]byte(`{"version":"4.0.0","files":["htmx.min.js","ext/json-enc.js"]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := New()
	client.apiBaseURL = server.URL + "/libraries/"

	files, version, err := client.FetchPackageFiles(t.Context(), "htmx", "4.0.0")
	if err != nil {
		t.Fatal(err)
	}

	if version != "4.0.0" || len(files) != 2 || files[1].LocalPath != "ext/json-enc.js" ||
		files[1].Path != "https://cdnjs.cloudflare.com/ajax/libs/htmx/4.0.0/ext/json-enc.js" {
		t.Fatalf("unexpected version or files: %q, %#v", version, files)
	}

	if strings.Join(paths, ",") != "/libraries/htmx/4.0.0" {
		t.Fatalf("requested wrong endpoint: %v", paths)
	}

	files, version, err = client.FetchPackageFiles(t.Context(), "htmx", "")
	if err != nil {
		t.Fatal(err)
	}
	if version != "4.0.0" || len(files) != 1 || files[0].LocalPath != "htmx.min.js" {
		t.Fatalf("unexpected latest version or files: %q, %#v", version, files)
	}
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

func TestFetchPackageFilesFallsBackToFilename(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/libraries/canvas-confetti":
			_, _ = w.Write([]byte(`{"version":"1.9.3","filename":"confetti.browser.min.js"}`))
		case "/libraries/canvas-confetti/1.9.3":
			_, _ = w.Write([]byte(`{"version":"1.9.3","filename":"confetti.browser.min.js","files":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	useDefaults(t, server.URL+"/libraries/", server.URL+"/ajax/libs/")

	// The zero Client uses the default URLs. Without a list of files the
	// package's main file is used, with and without a version.
	var client Client

	want := library.Files{{
		Type:      library.FileTypeJS,
		Path:      server.URL + "/ajax/libs/canvas-confetti/1.9.3/confetti.browser.min.js",
		LocalPath: "confetti.browser.min.js",
	}}

	for _, requested := range []string{"", "1.9.3"} {
		files, version, err := client.FetchPackageFiles(t.Context(), "canvas-confetti", requested)
		if err != nil {
			t.Fatal(err)
		}

		if version != "1.9.3" || !slices.Equal(files, want) {
			t.Errorf("FetchPackageFiles(%q) = %q, %+v, want 1.9.3, %+v", requested, version, files, want)
		}
	}
}

func TestFetchPackageFilesErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/libraries/status/1.0.0":
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		case "/libraries/garbage/1.0.0":
			_, _ = w.Write([]byte("<html>"))
		case "/libraries/htmx/4.0.0":
			// cdnjs answers an unknown version with another one.
			_, _ = w.Write([]byte(`{"version":"4.0.1","files":["htmx.min.js"]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	tests := []struct {
		name    string
		api     string
		pkgName string
		version string
		want    string
	}{
		{"invalid API URL", "http://example.invalid/\x7f/", "htmx", "4.0.0", "invalid control character"},
		{"unreachable API", "ftp://example.invalid/", "htmx", "4.0.0", "unsupported protocol scheme"},
		{"error status", server.URL + "/libraries/", "status", "1.0.0", "client api responded with code 503"},
		{"invalid JSON", server.URL + "/libraries/", "garbage", "1.0.0", "invalid character '<'"},
		{"other version", server.URL + "/libraries/", "htmx", "4.0.0", `cdnjs returned version "4.0.1" for requested version "4.0.0"`},
	}

	for _, tt := range tests {
		client := New()
		client.apiBaseURL = tt.api

		files, version, err := client.FetchPackageFiles(t.Context(), tt.pkgName, tt.version)
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: error = %v, want one containing %q", tt.name, err, tt.want)
		}

		if files != nil || version != "" {
			t.Errorf("%s: FetchPackageFiles = %q, %+v with an error, want nothing", tt.name, version, files)
		}
	}
}
