package importmap

import (
	"context"
	"log/slog"
	"testing"

	"github.com/donseba/go-importmap/client/cdnjs"
	"github.com/donseba/go-importmap/library"
)

func TestImportMapWithLocalAssets(t *testing.T) {
	ctx := context.Background()
	pr := cdnjs.New()

	im := New().
		WithDefaults().
		WithProvider(pr).
		WithPackages([]library.Package{
			{
				Name:    "htmx",
				Version: "1.9.10",
				Require: []library.Include{
					{
						File: "htmx.min.js",
					},
					{
						File: "/ext/json-enc.js",
						As:   "json-enc",
					},
				},
			},
			{
				Name: "bootstrap",
				Require: []library.Include{
					{
						File: "css/bootstrap.min.css",
					},
					{
						File: "js/bootstrap.min.js",
						As:   "bootstrap",
					},
				},
			},
		})

	err := im.Fetch(ctx)
	if err != nil {
		t.Error(err)
		return
	}

	out, err := im.Imports()
	if err != nil {
		t.Error(err)
		return
	}

	if string(out) != `{"imports":{"bootstrap":"/assets/js/bootstrap.min.js","htmx":"/assets/htmx.min.js","json-enc":"/assets/ext/json-enc.js"}}` {
		t.Error("json output mismatch")
		return
	}

	outStyles, err := im.Styles()
	if err != nil {
		t.Error(err)
		return
	}

	if string(outStyles) != `<link rel="stylesheet" href="/assets/css/bootstrap.min.css" as="bootstrap">` {
		t.Error("json output mismatch")
		return
	}

	full, err := im.Render()
	if err != nil {
		t.Error(err)
		return
	}

	t.Log(full)
	if string(full) != `<link rel="stylesheet" href="/assets/css/bootstrap.min.css" as="bootstrap"/>
<script async src="https://ga.jspm.io/npm:es-module-shims@1.7.0/dist/es-module-shims.js"></script>
<script type="importmap">
{
  "bootstrap": "/assets/js/bootstrap.min.js",
  "htmx": "/assets/htmx.min.js",
  "json-enc": "/assets/ext/json-enc.js"
}
</script>` {
		t.Error("json output mismatch")
		return
	}
}

func TestImportRaw(t *testing.T) {
	ctx := context.Background()
	pr := cdnjs.New()
	im := New().WithProvider(pr).WithLogger(slog.Default())

	im.WithPackages([]library.Package{
		{
			Name:    "htmx",
			Version: "1.8.6",
			Require: []library.Include{
				{
					Raw: "https://unpkg.com/browse/htmx.org@1.8.6/dist/htmx.min.js",
					As:  "htmx",
				},
			},
		},
	})

	err := im.Fetch(ctx)
	if err != nil {
		t.Error(err)
		return
	}

	out, err := im.Imports()
	if err != nil {
		t.Error(err)
		return
	}

	if string(out) != `{"imports":{"htmx":"https://unpkg.com/browse/htmx.org@1.8.6/dist/htmx.min.js"}}` {
		t.Error("json output mismatch")
		return
	}
}
