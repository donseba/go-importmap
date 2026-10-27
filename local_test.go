package importmap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donseba/go-importmap/assets"
	"github.com/donseba/go-importmap/library"
)

// writeLocal writes content to root/name.
func writeLocal(t *testing.T, root, name, content string) {
	t.Helper()

	filename := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestVendorLocalPackage(t *testing.T) {
	server, requests := vendorServer(t, map[string]string{"/htmx/htmx.esm.min.js": "export default {}"})
	provider := fixtureProvider{baseURL: server.URL, paths: map[string][]string{"htmx": {"htmx.esm.min.js"}}}

	root := t.TempDir()
	dir := filepath.Join(root, "vendor")
	writeLocal(t, root, "editor/bundle.js", "export const bundle = 1")
	writeLocal(t, root, "editor/LICENSES.txt", "MIT")

	im := New().WithProvider(provider).WithPackages([]library.Package{
		{Name: "htmx", Version: "4.0.0", Require: library.Includes{{File: "htmx.esm.min.js", As: "htmx"}}},
		{
			Name: "editor", Local: true, License: "../editor/LICENSES.txt",
			Require: library.Includes{{File: "../editor/bundle.js", As: "editor-bundle"}, {File: "./../editor/LICENSES.txt"}},
		},
	})

	lock, err := im.Vendor(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}

	local := lock.Packages[1]
	if !local.Local || local.Provider != "" || local.License != "../editor/LICENSES.txt" || len(local.Files) != 2 {
		t.Fatalf("local package = %+v", local)
	}

	if file := local.Files[0]; file.Path != "../editor/bundle.js" || file.As != "editor-bundle" || file.Source != "" ||
		file.Integrity != assets.Integrity([]byte("export const bundle = 1")) {
		t.Errorf("bundle = %+v", file)
	}

	data, _ := os.ReadFile(filepath.Join(dir, LockFile))
	if !strings.Contains(string(data), `"local": true`) || strings.Contains(string(data), `"source": ""`) {
		t.Errorf("lock = %s", data)
	}

	// FromFS checks the local files from a file system that holds them.
	loaded, err := FromFS(os.DirFS(root), "vendor/"+LockFile)
	if err != nil {
		t.Fatal(err)
	}

	if got := loaded.Structure.Imports["editor-bundle"]; !strings.HasPrefix(got, "/assets/editor/bundle.js?v=") {
		t.Errorf("bundle URL = %q", got)
	}

	if _, err := FromFS(os.DirFS(dir), LockFile); err == nil || !strings.Contains(err.Error(), "outside the file system") {
		t.Errorf("local files outside the file system: %v", err)
	}

	// A rebuilt bundle fails until it is locked again, without network.
	writeLocal(t, root, "editor/bundle.js", "export const bundle = 2")

	if _, err := FromFS(os.DirFS(root), "vendor/"+LockFile); err == nil || !strings.Contains(err.Error(), "integrity") {
		t.Errorf("changed bundle: %v", err)
	}

	before := requests("/htmx/htmx.esm.min.js")

	relocked, err := im.VendorLocal(dir)
	if err != nil {
		t.Fatal(err)
	}

	if requests("/htmx/htmx.esm.min.js") != before {
		t.Error("VendorLocal downloaded")
	}

	if relocked.Packages[0].Files[0].Integrity != lock.Packages[0].Files[0].Integrity {
		t.Error("VendorLocal changed a vendored package")
	}

	if _, err := FromFS(os.DirFS(root), "vendor/"+LockFile); err != nil {
		t.Errorf("after VendorLocal: %v", err)
	}

	// Dropping the local package never removes the application's files.
	im.WithPackages(im.packages[:1])

	if _, err := im.Vendor(t.Context(), dir); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(root, "editor", "bundle.js")); err != nil {
		t.Errorf("a local file was removed: %v", err)
	}
}

func TestVendorLocalErrors(t *testing.T) {
	dir := t.TempDir()

	missing := New().WithPackage(library.Package{Name: "editor", Local: true, Require: library.Includes{{File: "bundle.js"}}})
	if _, err := missing.Vendor(t.Context(), dir); err == nil {
		t.Error("a missing local file was locked")
	}

	empty := New().WithPackage(library.Package{Name: "editor", Local: true})
	if _, err := empty.Vendor(t.Context(), dir); err == nil {
		t.Error("a local package without files was locked")
	}

	absolute := New().WithPackage(library.Package{Name: "editor", Local: true, Require: library.Includes{{File: "/etc/passwd"}}})
	if _, err := absolute.Vendor(t.Context(), dir); err == nil {
		t.Error("an absolute path was locked")
	}

	writeLocal(t, dir, ".hidden.js", "1")

	hidden := New().WithPackage(library.Package{Name: "editor", Local: true, Require: library.Includes{{File: ".hidden.js"}}})
	if _, err := hidden.Vendor(t.Context(), dir); err == nil || !strings.Contains(err.Error(), `starts with "."`) {
		t.Errorf("a hidden local file: err = %v", err)
	}

	writeLocal(t, dir, "bundle.js", "1")

	unvendored := New().WithPackages([]library.Package{
		{Name: "htmx", Version: "4.0.0"},
		{Name: "editor", Local: true, Require: library.Includes{{File: "bundle.js"}}},
	})

	if _, err := unvendored.VendorLocal(dir); err == nil {
		t.Error("VendorLocal without a lock")
	}

	writeLocal(t, dir, LockFile, `{"lockfileVersion": 1, "packages": []}`)

	if _, err := unvendored.VendorLocal(dir); err == nil || !strings.Contains(err.Error(), "vendor the packages first") {
		t.Errorf("VendorLocal with a package not vendored: %v", err)
	}
}

func TestFetchSkipsLocalPackages(t *testing.T) {
	im := New().WithPackage(library.Package{Name: "editor", Local: true, Require: library.Includes{{File: "bundle.js"}}})

	if err := im.Fetch(t.Context()); err != nil {
		t.Errorf("Fetch: %v", err)
	}

	im.CacheDir(t.TempDir()).AssetsDir(t.TempDir())

	if err := im.CacheOrFetch(t.Context()); err != nil {
		t.Errorf("CacheOrFetch: %v", err)
	}
}
