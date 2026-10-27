package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donseba/go-importmap"
)

func TestVendorAndVerify(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lib.js":
			_, _ = w.Write([]byte("export const lib = 1"))
		case "/LICENSE":
			_, _ = w.Write([]byte("MIT"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	configFile := filepath.Join(dir, "importmap.json")
	out := filepath.Join(dir, "vendor")

	config := `{"packages": [
		{"name": "lib.js", "url": "` + server.URL + `/lib.js", "as": "lib", "license": "` + server.URL + `/LICENSE"}
	]}`
	if err := os.WriteFile(configFile, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := run(t.Context(), []string{"vendor", "-config", configFile, "-out", out}, &stdout); err != nil {
		t.Fatal(err)
	}

	if got := stdout.String(); got != "lib.js: 2 files (lib.js/LICENSE)\n" {
		t.Errorf("output = %q", got)
	}

	im, err := importmap.FromFS(os.DirFS(out), importmap.LockFile)
	if err != nil {
		t.Fatal(err)
	}

	if got := im.Structure.Imports["lib"]; !strings.HasPrefix(got, "/assets/lib.js/lib.js?v=") {
		t.Errorf("import = %q", got)
	}

	stdout.Reset()

	if err := run(t.Context(), []string{"verify", "-out", out}, &stdout); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(out, "lib.js", "lib.js"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := run(t.Context(), []string{"verify", "-out", out}, &stdout); err == nil {
		t.Fatal("verify accepted a changed file")
	}
}

func TestConfigErrors(t *testing.T) {
	dir := t.TempDir()

	for name, config := range map[string]string{
		"unknown field":    `{"packages": [{"name": "x", "versoin": "1"}]}`,
		"unknown provider": `{"provider": "npm", "packages": [{"name": "x"}]}`,
		"no packages":      `{"packages": []}`,
		"no name":          `{"packages": [{"version": "1"}]}`,
		"local with url":   `{"packages": [{"name": "x", "local": true, "url": "https://example.com/x.js"}]}`,
	} {
		configFile := filepath.Join(dir, "importmap.json")
		if err := os.WriteFile(configFile, []byte(config), 0o644); err != nil {
			t.Fatal(err)
		}

		if err := run(t.Context(), []string{"vendor", "-config", configFile, "-out", dir}, &bytes.Buffer{}); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}

	for _, args := range [][]string{nil, {"unknown"}, {"vendor", "-nope"}} {
		if err := run(t.Context(), args, &bytes.Buffer{}); err == nil {
			t.Errorf("%v: accepted", args)
		}
	}
}

func TestLocalPackage(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, "vendor")
	configFile := filepath.Join(root, "importmap.json")

	if err := os.MkdirAll(filepath.Join(root, "editor"), 0o755); err != nil {
		t.Fatal(err)
	}

	bundle := filepath.Join(root, "editor", "bundle.js")
	if err := os.WriteFile(bundle, []byte("export const bundle = 1"), 0o644); err != nil {
		t.Fatal(err)
	}

	config := `{"packages": [{"name": "editor", "local": true, "require": [{"file": "../editor/bundle.js", "as": "bundle"}]}]}`
	if err := os.WriteFile(configFile, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := run(t.Context(), []string{"vendor", "-config", configFile, "-out", out}, &stdout); err != nil {
		t.Fatal(err)
	}

	if got := stdout.String(); got != "editor: 1 local files (no licence file found)\n" {
		t.Errorf("output = %q", got)
	}

	if err := run(t.Context(), []string{"verify", "-out", out}, &stdout); err == nil {
		t.Error("verify read local files outside -out without -root")
	}

	if err := run(t.Context(), []string{"verify", "-out", out, "-root", root}, &stdout); err != nil {
		t.Fatal(err)
	}

	if err := run(t.Context(), []string{"verify", "-out", root, "-root", out}, &stdout); err == nil {
		t.Error("verify accepted -out outside -root")
	}

	if err := os.WriteFile(bundle, []byte("export const bundle = 2"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := run(t.Context(), []string{"verify", "-out", out, "-root", root}, &stdout); err == nil {
		t.Fatal("verify accepted a rebuilt bundle")
	}

	if err := run(t.Context(), []string{"vendor", "-config", configFile, "-out", out, "-local"}, &stdout); err != nil {
		t.Fatal(err)
	}

	if err := run(t.Context(), []string{"verify", "-out", out, "-root", root}, &stdout); err != nil {
		t.Errorf("after vendor -local: %v", err)
	}
}
