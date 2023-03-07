package importmap

import (
	"context"
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

	if string(out) != `{"imports":{"htmx":"https://cdnjs.cloudflare.com/ajax/libs/htmx/1.8.6/htmx.min.js","json-enc":"https://cdnjs.cloudflare.com/ajax/libs/htmx/1.8.6/ext/json-enc.min.js"}}` {
		t.Error("json output mismatch")
		return
	}

	tmpl, err := im.Render()
	if err != nil {
		t.Error(err)
		return
	}

	t.Log(tmpl)
}
