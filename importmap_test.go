package importmap

import (
	"log/slog"
	"testing"

	"github.com/donseba/go-importmap/client/cdnjs"
	"github.com/donseba/go-importmap/client/jsdelivr"
	"github.com/donseba/go-importmap/client/raw"
	"github.com/donseba/go-importmap/library"
)

func TestImportMapWithLocalAssets(t *testing.T) {
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

	err := im.Fetch(t.Context())
	if err != nil {
		t.Error(err)
		return
	}

	out, err := im.Imports()
	if err != nil {
		t.Error(err)
		return
	}

	if string(out) != `{"imports":{"bootstrap":"/assets/bootstrap/js/bootstrap.min.js","htmx":"/assets/htmx/htmx.min.js","json-enc":"/assets/htmx/ext/json-enc.js"}}` {
		t.Log(out)
		t.Error("json output mismatch")
		return
	}

	outStyles, err := im.Styles()
	if err != nil {
		t.Error(err)
		return
	}

	if string(outStyles) != `<link rel="stylesheet" href="/assets/bootstrap/css/bootstrap.min.css" as="bootstrap">` {
		t.Log(outStyles)
		t.Error("json output mismatch")
		return
	}

	full, err := im.Render()
	if err != nil {
		t.Error(err)
		return
	}

	if string(full) != `<link rel="stylesheet" href="/assets/bootstrap/css/bootstrap.min.css" as="bootstrap"/>
<script async src="https://ga.jspm.io/npm:es-module-shims@2.0.10/dist/es-module-shims.js"></script>
<script type="importmap">
{
  "imports": {
    "bootstrap": "/assets/bootstrap/js/bootstrap.min.js",
    "htmx": "/assets/htmx/htmx.min.js",
    "json-enc": "/assets/htmx/ext/json-enc.js"
  }
}
</script>` {
		t.Error("json output mismatch")
		return
	}
}

func TestImportMapWithLocalAssetsJsDeliver(t *testing.T) {
	pr := jsdelivr.New()

	im := New().
		WithDefaults().
		WithProvider(pr).
		WithPackages([]library.Package{
			{
				Name: "htmx.org",
				Require: []library.Include{
					{
						File: "*/htmx.min.js",
					},
					{
						File: "*/json-enc.js",
						As:   "json-enc",
					},
				},
			},
			{
				Name: "bootstrap",
				Require: []library.Include{
					{
						File: "/dist**bootstrap.min.css",
					},
					{
						File: "/dist**bootstrap.min.js",
						As:   "bootstrap",
					},
				},
			},
		})

	err := im.Fetch(t.Context())
	if err != nil {
		t.Error(err)
		return
	}

	out, err := im.Imports()
	if err != nil {
		t.Error(err)
		return
	}

	if string(out) != `{"imports":{"bootstrap":"/assets/bootstrap/dist/js/bootstrap.min.js","htmx":"/assets/htmx.org/dist/htmx.min.js","json-enc":"/assets/htmx.org/dist/ext/json-enc.js"}}` {
		t.Log(out)
		t.Error("json output mismatch")
		return
	}

	outStyles, err := im.Styles()
	if err != nil {
		t.Error(err)
		return
	}

	if string(outStyles) != `<link rel="stylesheet" href="/assets/bootstrap/dist/css/bootstrap.min.css" as="bootstrap">` {
		t.Log(outStyles)
		t.Error("json output mismatch")
		return
	}

	full, err := im.Render()
	if err != nil {
		t.Error(err)
		return
	}

	if string(full) != `<link rel="stylesheet" href="/assets/bootstrap/dist/css/bootstrap.min.css" as="bootstrap"/>
<script async src="https://ga.jspm.io/npm:es-module-shims@2.0.10/dist/es-module-shims.js"></script>
<script type="importmap">
{
  "imports": {
    "bootstrap": "/assets/bootstrap/dist/js/bootstrap.min.js",
    "htmx": "/assets/htmx.org/dist/htmx.min.js",
    "json-enc": "/assets/htmx.org/dist/ext/json-enc.js"
  }
}
</script>` {
		t.Log(full)
		t.Error("json output mismatch")
		return
	}
}

func TestImportRaw(t *testing.T) {
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

	err := im.Fetch(t.Context())
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

func TestImportRawClient(t *testing.T) {
	pr := cdnjs.New()
	im := New().WithProvider(pr).WithLogger(slog.Default())

	im.WithPackages([]library.Package{
		{
			Name:     "htmx",
			Version:  "2.0.4",
			Provider: raw.New("https://unpkg.com/browse/htmx.org@2.0.4/dist/htmx.min.js"),
		},
	})

	err := im.Fetch(t.Context())
	if err != nil {
		t.Error(err)
		return
	}

	out, err := im.Imports()
	if err != nil {
		t.Error(err)
		return
	}

	if string(out) != `{"imports":{"htmx":"https://unpkg.com/browse/htmx.org@2.0.4/dist/htmx.min.js"}}` {
		t.Error("json output mismatch")
		return
	}
}
