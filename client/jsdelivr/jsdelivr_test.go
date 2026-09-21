package jsdelivr

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
