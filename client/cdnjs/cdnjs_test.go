package cdnjs

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchPackageFilesUsesRequestedVersion(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/libraries/htmx":
			_, _ = w.Write([]byte(`{"version":"4.0.0","filename":"htmx.min.js","assets":[{"version":"4.0.0","files":["htmx.min.js"]}]}`))
		case "/libraries/htmx/1.8.6":
			_, _ = w.Write([]byte(`{"version":"1.8.6","files":["htmx.min.js","ext/json-enc.js"]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := New()
	client.apiBaseURL = server.URL + "/libraries/"
	files, version, err := client.FetchPackageFiles(t.Context(), "htmx", "1.8.6")
	if err != nil {
		t.Fatal(err)
	}
	if version != "1.8.6" || len(files) != 2 || files[1].LocalPath != "ext/json-enc.js" ||
		files[1].Path != "https://cdnjs.cloudflare.com/ajax/libs/htmx/1.8.6/ext/json-enc.js" {
		t.Fatalf("unexpected version or files: %q, %#v", version, files)
	}
	if strings.Join(paths, ",") != "/libraries/htmx/1.8.6" {
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
