package importmap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/donseba/go-importmap/library"
)

func TestAssetDownloadsRejectHTTPFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "asset not found", http.StatusNotFound)
	}))
	defer server.Close()
	for _, cached := range []bool{false, true} {
		root := t.TempDir()
		pkg := library.Package{Name: "demo", Version: "1.0.0"}
		var err error
		if cached {
			err = pkg.MakeCache(root, ".importmap", "demo.js", server.URL)
		} else {
			err = pkg.MakeAssets(root, "", "assets", "demo.js", server.URL)
		}
		if err == nil || !strings.Contains(err.Error(), "404") {
			t.Errorf("cached=%v accepted HTTP error: %v", cached, err)
		}
		for _, filename := range []string{filepath.Join(root, pkg.AssetsDir("assets"), "demo.js"), filepath.Join(root, pkg.CacheDir(".importmap"), "demo.js")} {
			if _, err := os.Stat(filename); !os.IsNotExist(err) {
				t.Errorf("HTTP error published a file at %s: %v", filename, err)
			}
		}
	}
}

func TestFailedDownloadPreservesExistingFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1024")
		_, _ = fmt.Fprint(w, "incomplete")
	}))
	defer server.Close()
	root := t.TempDir()
	pkg := library.Package{Name: "demo", Version: "1.0.0"}
	filename := filepath.Join(root, pkg.CacheDir(".importmap"), "demo.js")
	if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte("complete"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := pkg.MakeCache(root, ".importmap", "demo.js", server.URL); err == nil {
		t.Fatal("accepted incomplete download")
	}
	content, err := os.ReadFile(filename)
	if err != nil || string(content) != "complete" {
		t.Fatalf("failed download replaced the complete file: %s %v", content, err)
	}
	if temporary, _ := filepath.Glob(filepath.Join(filepath.Dir(filename), ".importmap-*")); len(temporary) != 0 {
		t.Fatalf("temporary files left behind: %v", temporary)
	}
}

func TestFetchDownloadUsesCallerContext(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = fmt.Fprint(w, "fixture")
	}))
	defer server.Close()
	im := NewDefaults().RootDir(t.TempDir()).WithProvider(fixtureProvider{baseURL: server.URL, paths: map[string][]string{"demo": {"demo.js"}}}).WithPackages([]library.Package{{Name: "demo", Version: "1.0.0"}})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := im.Fetch(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled fetch error = %v", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("cancelled fetch made %d HTTP requests", requests.Load())
	}
}

func TestFetchRetriesIncompleteCache(t *testing.T) {
	var firstRequests, secondRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "second.js") {
			if secondRequests.Add(1) == 1 {
				http.Error(w, "retry later", http.StatusServiceUnavailable)
				return
			}
		} else {
			firstRequests.Add(1)
		}
		_, _ = fmt.Fprint(w, "complete")
	}))
	defer server.Close()
	root := t.TempDir()
	im := NewDefaults().RootDir(root).WithProvider(fixtureProvider{baseURL: server.URL, paths: map[string][]string{"demo": {"first.js", "second.js"}}}).WithPackages([]library.Package{{Name: "demo", Version: "1.0.0"}})
	if err := im.Fetch(t.Context()); err == nil {
		t.Fatal("accepted failed first download")
	}
	if err := im.Fetch(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"first.js", "second.js"} {
		content, err := os.ReadFile(filepath.Join(root, "assets", "demo", name))
		if err != nil || string(content) != "complete" {
			t.Fatalf("retry failed to publish %s: %s %v", name, content, err)
		}
	}
	if firstRequests.Load() != 1 || secondRequests.Load() != 2 {
		t.Fatalf("retry downloads = %d/%d", firstRequests.Load(), secondRequests.Load())
	}
}
