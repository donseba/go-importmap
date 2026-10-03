<p align="center">
    <a href="https://docs.gowebthings.com/go-importmap">
        <img src="./assets/go-importmap-logo.png" alt="go-importmap" height="70">
    </a>
</p>

# go-importmap

[Documentation](https://docs.gowebthings.com/go-importmap) · Part of [go-webthings](https://gowebthings.com/components).

go-importmap fetches JavaScript and CSS from CDNs, caches package files locally, and generates import maps and stylesheet tags for Go templates. It handles asset preparation; your application serves the generated files and decides which modules to import.

## Installation

```sh
go get github.com/donseba/go-importmap
```

## Quick start

Run this during an asset build or application startup, before serving requests:

```go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/donseba/go-importmap"
	"github.com/donseba/go-importmap/library"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	im := importmap.NewDefaults().
		WithPackages([]library.Package{
			{
				Name: "htmx", Version: "2.0.4",
				Require: []library.Include{
					{File: "htmx.esm.min.js", As: "htmx"},
				},
			},
			{
				Name: "bootstrap", Version: "5.3.3",
				Require: []library.Include{
					{File: "css/bootstrap.min.css", As: "bootstrap"},
				},
			},
		})
	if err := im.CacheOrFetch(ctx); err != nil {
		log.Fatal(err)
	}
	head, err := im.Render()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(head)
}
```

The generated head contains a stylesheet link to `/assets/bootstrap/css/bootstrap.min.css` and an import map mapping `htmx` to `/assets/htmx/htmx.esm.min.js`. It also includes the configured ES module shim. Import the module after the map:

```html
<script type="module">
    import htmx from "htmx";
    window.htmx = htmx;
</script>
```

An import map maps names to URLs; it does not execute modules. Use ESM files for module imports and ordinary script tags for libraries distributed as classic scripts.

## Serving local assets

With the defaults, serve the generated `assets` directory at `/assets/`:

```go
mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("assets"))))
```

`RootDir` controls filesystem reads and writes, while `AssetsDir` controls the asset directory and public URL prefix. For example, `RootDir("/srv/my-app").AssetsDir("assets")` writes below `/srv/my-app/assets`, while URLs still begin with `/assets/`. Serve that absolute directory if your working directory is elsewhere.

Pin package versions, prepare assets before deployment, and ship the generated files with your application. Cache files live below `.importmap/<package>/<version>/`; public assets live below `assets/<package>/`. `CacheOrFetch` can rebuild missing assets from a complete cache without contacting the provider. Keep package configuration fixed once serving begins.

## Providers and configuration

| Provider | Import path | Constructor |
|----------|-------------|-------------|
| cdnjs (default) | `client/cdnjs` | `cdnjs.New()` |
| jsDelivr | `client/jsdelivr` | `jsdelivr.New()` or `jsdelivr.NewESM()` |
| unpkg | `client/unpkg` | `unpkg.New()` |
| Skypack | `client/skypack` | `skypack.New()` |
| esm.sh | `client/esm` | `esm.New()` |
| Raw URL | `client/raw` | `raw.New(url)` |

Provider paths are relative to `github.com/donseba/go-importmap`. Implement `library.Provider` to add your own. Set a default with `WithProvider`, or override it through `library.Package.Provider`.

- `NewDefaults()` creates an import map with cdnjs, `assets`, `.importmap`, and an ES module shim; `New().WithDefaults()` is equivalent.
- `WithPackages` replaces the configured list; `WithPackage` appends one package.
- `library.Include.File` selects a provider file; `As` gives it an import name.
- `CacheDir`, `AssetsDir`, and `RootDir` set storage paths; `ShimPath` sets the shim URL.
- `WithLogger` adds a `*slog.Logger`.
- `Fetch` contacts providers; `CacheOrFetch` reuses cached files when possible.
- `Render` produces the head HTML; `Imports` produces import-map JSON; `Marshal` includes imports, scopes, and styles.

Downloads use the supplied context, reject unsuccessful HTTP responses, and publish files only after a complete download. Retrying a failed fetch fills missing files without downloading complete files again.

## Raw URLs

For a single URL to download and cache locally, configure the package's `Provider` with `raw.New(url)` from `github.com/donseba/go-importmap/client/raw`. The raw provider uses the package name as the local filename; give that name a `.js` or `.css` extension so rendering can recognize the asset type. Public files still live below the package's asset directory.

## License

Distributed under the MIT License. See [LICENSE](LICENSE).
