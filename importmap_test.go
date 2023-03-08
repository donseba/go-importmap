package importmap

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/donseba/go-importmap/client/cdnjs"
	"github.com/donseba/go-importmap/library"
)

func TestImportMap(t *testing.T) {
	ctx := context.Background()
	pr := cdnjs.New()
	im := New(pr)

	im.Packages = []library.Package{
		{
			Name:    "htmx",
			Version: "1.8.6",
		},
		{
			Name:     "htmx",
			Version:  "1.8.6",
			As:       "json-enc",
			FileName: "ext/json-enc.min.js",
		},
		{
			Name:    "htmx",
			Version: "1.8.5",
			As:      "json-enc",
		},
	}

	err := im.Fetch(ctx)
	if err != nil {
		t.Error(err)
		return
	}

	out, err := im.Marshal()
	if err != nil {
		t.Error(err)
		return
	}

	if string(out) != `{"imports":{"htmx":"https://cdnjs.cloudflare.com/ajax/libs/htmx/1.8.6/htmx.min.js","json-enc":"https://cdnjs.cloudflare.com/ajax/libs/htmx/1.8.5/htmx.min.js"}}` {
		t.Error("json output mismatch")
		return
	}
}

func TestImportMapCache(t *testing.T) {
	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	exPath := filepath.Dir(ex)

	ctx := context.Background()
	pr := cdnjs.New()
	im := New(pr)
	im.SetAssetsDir(defaultAssetsDir)
	im.SetAssetsPath(defaultAssetsPath)
	im.SetCacheDir(defaultCacheDir)
	im.SetShimSrc(defaultShimSrc)
	im.SetRootDir(exPath)
	im.SetUseAssets(true)
	im.SetIncludeShim(true)
	im.SetClean(true)

	im.Packages = []library.Package{
		{
			Name:    "htmx",
			Version: "1.8.6",
		},
		{
			Name:     "htmx",
			Version:  "1.8.6",
			As:       "json-enc",
			FileName: "ext/json-enc.min.js",
		},
		{
			Name:    "htmx",
			Version: "1.8.5",
			As:      "json-enc",
		},
	}

	err = im.Fetch(ctx)
	if err != nil {
		t.Error(err)
		return
	}

	out, err := im.Marshal()
	if err != nil {
		t.Error(err)
		return
	}

	if string(out) != `{"imports":{"htmx":"/useAssets/js/htmx/htmx.min.js","json-enc":"/useAssets/js/htmx/htmx.min.js"}}` {
		t.Error("json output mismatch")
		return
	}

	_, err = im.Imports()
	if err != nil {
		t.Error(err)
		return
	}

	_, err = im.Render()
	if err != nil {
		t.Error(err)
		return
	}

	_, err = im.MarshalIndent()
	if err != nil {
		t.Error(err)
		return
	}
}

func TestImportMapRaw(t *testing.T) {
	ctx := context.Background()
	pr := cdnjs.New()
	im := New(pr)

	im.Packages = []library.Package{
		{
			Name: "htmx",
			Raw:  "https://some.url.to/repo/with.js",
		},
	}

	err := im.Fetch(ctx)
	if err != nil {
		t.Error(err)
		return
	}

	out, err := im.Marshal()
	if err != nil {
		t.Error(err)
		return
	}

	if string(out) != `{"imports":{"htmx":"https://some.url.to/repo/with.js"}}` {
		t.Error("json output mismatch")
		return
	}
}

func TestImportMapRawPublish(t *testing.T) {
	ctx := context.Background()
	pr := cdnjs.New()
	im := New(pr)
	im.SetUseAssets(true)

	im.Packages = []library.Package{
		{
			Name:    "htmx",
			Raw:     "https://unpkg.com/browse/htmx.org@1.8.6/dist/htmx.min.js",
			Version: "1.8.6",
		},
	}

	err := im.Fetch(ctx)
	if err != nil {
		t.Error(err)
		return
	}

	out, err := im.Marshal()
	if err != nil {
		t.Error(err)
		return
	}

	if string(out) != `{"imports":{"htmx":"/useAssets/js/htmx/htmx.min.js"}}` {
		t.Error("json output mismatch")
		return
	}
}
