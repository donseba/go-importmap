package raw

import (
	"encoding/json"
	"testing"

	"github.com/donseba/go-importmap/library"
)

func TestNew(t *testing.T) {
	cdn := New("https://unpkg.com/browse/htmx.org@4.0.0/dist/htmx.min.js")

	f, v, err := cdn.FetchPackageFiles(t.Context(), "htmx.org", "4.0.0")
	if err != nil {
		t.Error(err)
		return
	}

	if v != "4.0.0" {
		t.Error("version mismatch")
	}

	if len(f) == 0 {
		t.Error("no files found")
	}

	out, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		t.Error(err)
		return
	}

	t.Log(string(out))
}

func TestFetchPackageFilesWithoutURL(t *testing.T) {
	files, version, err := New("").FetchPackageFiles(t.Context(), "htmx.org", "4.0.0")
	if err == nil || err.Error() != "raw provider URL is empty" {
		t.Fatalf("error = %v, want raw provider URL is empty", err)
	}

	if files != nil || version != "" {
		t.Fatalf("FetchPackageFiles = %q, %+v with an error, want nothing", version, files)
	}
}

func TestFetchPackageFilesKeepsURLAndVersion(t *testing.T) {
	files, version, err := New("https://cdn.example/htmx.org/dist/htmx.min.css").FetchPackageFiles(t.Context(), "htmx-styles", "")
	if err != nil {
		t.Fatal(err)
	}

	want := library.File{Type: library.FileTypeCSS, Path: "https://cdn.example/htmx.org/dist/htmx.min.css", LocalPath: "htmx-styles"}
	if version != "" || len(files) != 1 || files[0] != want {
		t.Fatalf("FetchPackageFiles = %q, %+v, want no version and %+v", version, files, want)
	}
}
