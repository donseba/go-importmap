package importmap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donseba/go-importmap/library"
)

type offlineProvider struct{}

func (offlineProvider) FetchPackageFiles(context.Context, string, string) (library.Files, string, error) {
	return nil, "", fmt.Errorf("provider must not be used when loading the cache")
}

func TestRootDirFetchAndOfflineCache(t *testing.T) {
	t.Chdir(t.TempDir())
	root := t.TempDir()
	server := assetServer(t)
	newMap := func(provider library.Provider) *ImportMap {
		return NewDefaults().RootDir(root).ShimPath("").WithProvider(provider).WithPackages([]library.Package{{
			Name: "demo", Version: "1.2.3",
			Require: library.Includes{{File: "nested/demo.js", As: "demo"}, {File: "nested/demo.css", As: "demo-css"}},
		}})
	}
	fetched := newMap(fixtureProvider{baseURL: server.URL, paths: map[string][]string{"demo": {"nested/demo.js", "nested/demo.css"}}})
	if err := fetched.Fetch(t.Context()); err != nil {
		t.Fatal(err)
	}
	check := func(im *ImportMap) {
		t.Helper()
		if got := im.Structure.Imports["demo"]; got != "/assets/demo/nested/demo.js" {
			t.Errorf("browser JS URL = %q", got)
		}
		if got := im.Structure.Styles["demo-css"]; got != "/assets/demo/nested/demo.css" {
			t.Errorf("browser CSS URL = %q", got)
		}
		for _, name := range []string{"demo.js", "demo.css"} {
			content, err := os.ReadFile(filepath.Join(root, "assets", "demo", "nested", name))
			if err != nil || string(content) != "fixture" {
				t.Fatalf("asset %s missing or corrupt: %s %v", name, content, err)
			}
		}
		out, err := im.Render()
		if err != nil || strings.Contains(string(out), root) {
			t.Fatalf("filesystem root leaked into markup: %s %v", out, err)
		}
	}
	check(fetched)
	server.Close()
	loaded := newMap(offlineProvider{})
	if err := loaded.CacheOrFetch(t.Context()); err != nil {
		t.Fatal(err)
	}
	check(loaded)
	if err := os.RemoveAll(filepath.Join(root, "assets")); err != nil {
		t.Fatal(err)
	}
	rebuilt := newMap(offlineProvider{})
	if err := rebuilt.CacheOrFetch(t.Context()); err != nil {
		t.Fatal(err)
	}
	check(rebuilt)
	if _, err := os.Stat(filepath.Join(root, "assets", "demo", "1.2.3")); !os.IsNotExist(err) {
		t.Fatalf("cache version became an asset path: %v", err)
	}
}

func TestAssetDirectoryStartingWithHHasAbsoluteURL(t *testing.T) {
	t.Chdir(t.TempDir())
	server := assetServer(t)
	im := NewDefaults().AssetsDir("hosted-assets").WithProvider(fixtureProvider{baseURL: server.URL, paths: map[string][]string{"demo": {"demo.js"}}}).WithPackages([]library.Package{{Name: "demo", Version: "1.0.0"}})
	if err := im.Fetch(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := im.Structure.Imports["demo.js"]; got != "/hosted-assets/demo/demo.js" {
		t.Fatalf("asset URL = %q", got)
	}
}
