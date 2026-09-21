package unpkg

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

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
