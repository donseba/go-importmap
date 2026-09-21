package skypack

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
