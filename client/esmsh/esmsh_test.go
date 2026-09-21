package esmsh

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
