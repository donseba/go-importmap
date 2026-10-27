<p align="center">
    <a href="https://docs.gowebthings.com/go-importmap">
        <img src="./assets/go-importmap-logo.png" alt="go-importmap" height="70">
    </a>
</p>

# go-importmap

[Documentation](https://docs.gowebthings.com/go-importmap) · Part of [go-webthings](https://gowebthings.com/components).

go-importmap fetches JavaScript and CSS packages from CDNs and generates import maps and stylesheet tags for Go templates. Packages can be vendored at build time and embedded in the program, so it needs no network at run time, and their files are served with versioned URLs, strong ETags and Subresource Integrity.

## Installation

```sh
go get github.com/donseba/go-importmap
```

## Two ways to use it

- **Vendor at build time, embed at run time** (recommended). The `importmap` command downloads the packages into a directory with their licences and a lock file. The program embeds that directory and loads it with `FromFS`: no network, no disk access.
- **Fetch at start-up.** `CacheOrFetch` downloads missing packages into `.importmap/`, publishes them in `assets/` and maps them. Handy while developing.

## Vendoring at build time

List the packages in `importmap.json`:

```json
{
  "provider": "cdnjs",
  "packages": [
    {
      "name": "htmx",
      "version": "4.0.0",
      "require": [{"file": "htmx.esm.min.js", "as": "htmx"}],
      "license": "https://unpkg.com/htmx.org@4.0.0/LICENSE"
    }
  ]
}
```

and vendor them, for example with `go generate`:

```go
//go:generate go run github.com/donseba/go-importmap/cmd/importmap vendor -config importmap.json -out assets/vendor
```

This writes `assets/vendor/htmx/htmx.esm.min.js`, `assets/vendor/htmx/LICENSE` and `assets/vendor/importmap.lock.json`, which lists every file with its source URL and SHA-384 integrity. Commit them. Running the command again downloads only what changed and removes files the configuration no longer requires. A lock that lists a downloaded file outside the out directory, such as `../x.js` after a hand edit, is refused before anything is downloaded or removed. `-timeout` sets the time allowed for all downloads (default `2m`). `importmap verify -out assets/vendor` checks the files against the lock without network, for CI.

| Field | Meaning |
|-------|---------|
| `provider` | Default provider: `cdnjs` (default), `jsdelivr`, `jsdelivr-esm`, `unpkg`, `skypack` or `esmsh`; a package can set its own |
| `name`, `version` | The package; pin the version |
| `require` | Files to vendor, as patterns (`file`) with an import name (`as`); `raw` maps a name to a URL left on its host. Without `require` every file is vendored. `file` is a path in the package; `**` matches any part of it, slashes included |
| `url` | A single file to download, saved under `name`; `as` names it |
| `license` | A file of the package or a URL with its licence text. Without it a `LICENSE`, `LICENCE` or `COPYING` file of the package is copied; the command reports packages without one |
| `local` | `true` for files the application builds itself (see below) |

`ImportMap.Vendor(ctx, dir)` does the same from Go.

### Built files in the lock

Files the application builds itself, such as a bundle made with esbuild, can be locked too, so `FromFS` and `importmap verify` check them like vendored files. A package with `"local": true` is not downloaded: `require` lists its files by their exact paths relative to the out directory, which may start with `../`, and `license` is such a path too:

```json
{"name": "editor", "local": true, "require": [{"file": "../editor/bundle.js"}], "license": "../editor/LICENSES.txt"}
```

`vendor` records their integrity in the lock (`"local": true`, without a source). Give a local file `as` to map it; without `as` it is only checked against the lock. After a rebuild, `importmap vendor -local` records it again without network, leaving the other packages' entries as they are (`ImportMap.VendorLocal(dir)` in Go). Local files are never removed. Files outside the out directory are only in a file system that holds both, so load the lock from that one, `FromFS(assetsFS, "vendor/importmap.lock.json")`, and verify with its root: `importmap verify -out assets/vendor -root assets`. In Go, `library.Package.Local` marks such a package; `Fetch` and `CacheOrFetch` leave it out. `library.Package.License` is the `license` setting of any package: a file of the package or a URL whose text `Vendor` copies, rather than a `LICENSE`, `LICENCE` or `COPYING` file it finds among the package's files.

At run time, embed the directory and load it:

```go
//go:embed assets/vendor
var vendored embed.FS

func importMap() (*importmap.ImportMap, error) {
	files, err := fs.Sub(vendored, "assets/vendor")
	if err != nil {
		return nil, err
	}

	return importmap.FromFS(files, importmap.LockFile)
}
```

A `go:embed` directory pattern leaves out files whose names start with `_`; write `//go:embed all:assets/vendor` when a package has such files.

`FromFS` checks every locked file against its integrity, so a hand-edited file fails at start-up. It fingerprints all files of the file system, not only the locked ones, so the application's own scripts and stylesheets in the same file system get versioned URLs too (see `URL` and `WithFile`). Files with a part that starts with a dot, such as `.github/` or `.DS_Store`, are left out, and `vendor` does not download or lock them; a local package that lists one fails.

`ReadLock(fsys, name)` reads a lock file alone, as a `*Lock` with its packages and files (path, import name, source and integrity), for tools that inspect what was vendored. It fails for a lock of an unknown `lockfileVersion`.

## Serving and URLs

`ServeFile(w, r, file)` serves the files of `FromFS` or `WithAssets` by name, from a route that passes the file name as a path parameter:

```go
mux.HandleFunc("GET /assets/{file...}", func(w http.ResponseWriter, r *http.Request) {
	im.ServeFile(w, r, r.PathValue("file"))
})
```

URLs are `/assets/<file>` by default (the assets directory, see `AssetsDir`). When the files are served elsewhere, for example by a named route of go-router (`r`), give the URL with `WithURLFunc`:

```go
im.WithURLFunc(func(file string) string {
	return r.RoutePathWithParams("assets", map[string]any{"file": file})
})
```

With `FromFS` or `WithAssets`, URLs carry the file's version (`?v=3f2a…`). `ServeFile` answers a request for the current version with `Cache-Control: public, max-age=31536000, immutable`, and any other request with `no-cache` and a strong `ETag`, so browsers revalidate cheaply.

`Manifest()` returns the fingerprints `FromFS` or `WithAssets` hold (`Version`, `Integrity`, `Files`, `ServeFile`), so there is no need to fingerprint the same files again.

A file whose size or modification time changed since it was fingerprinted, such as one edited on disk, is hashed again when served, so it is never sent under a version or `ETag` it no longer has: a request for the old version gets the new content with `no-cache` and the new `ETag`.

### Files loaded by relative paths

A script that loads another file relative to its own URL, such as `import "./chunk.js"` or `import.meta.resolve("./icons.svg")`, asks for the plain URL, without a version. `WithVersionedPaths` maps such plain URLs to the versioned ones in the import map, so those files can be cached for a year too:

```go
im.WithVersionedPaths()                         // every local file but the lock and licences
im.WithVersionedPaths("editor/prosemirror.js") // only these
im.WithVersionedPaths("**/*.js", "**/*.svg")   // files that match
im.WithVersionedPaths("!vendor/**")             // every file but these
```

The entries follow `WithURLFunc`. Without patterns it maps every file of the manifest, including files added later to one made with `assets.Reload`, except the lock file (`importmap.lock.json`) and licence texts (`LICENSE`, `LICENCE`, `COPYING`, `NOTICE`, with or without an extension), which no script loads. A pattern matches a file's whole path with the syntax of `path.Match`, where `**` stands for any number of directories; a pattern that starts with `!` leaves out the files it matches, and with only such patterns the rest is every file but the lock and licences. A name mapped with `WithImport` or `WithFile` wins. The entries are derived when the map renders (`Render`, `Marshal`, `Imports`); `Structure` holds what was configured.

The [`assets`](assets) package does this for any file system, with or without import maps:

```go
manifest, err := assets.Fingerprint(static) // e.g. an embed.FS
manifest.URL("site.css")                    // "/site.css?v=…"
manifest.Integrity("site.css")              // "sha384-…"
manifest.ServeFile(w, r, "site.css")        // answers a request for site.css
manifest.Stylesheet("site.css")             // <link rel="stylesheet" href="/site.css?v=…" integrity="…" crossorigin="anonymous">
manifest.PlainURL("site.css")               // "/site.css", the address without a version
```

`PlainURL` is the address a script reaches by a relative path; `URL` adds the version. `Stylesheet` fails for a file the manifest does not have.

`assets.ScriptTag(src, integrity, attrs...)` and `assets.StylesheetTag(href, integrity, attrs...)` write a `<script>` or stylesheet tag from an address and an integrity value, such as the manifest's `URL` or a route of your own; an empty integrity writes neither `integrity` nor `crossorigin`. Take the value from `manifest.TagIntegrity(file)`, which is `Integrity` except while the manifest reloads (below):

```go
tag, err := assets.ScriptTag(myRoute("app.js"), manifest.TagIntegrity("app.js"), "type", "module")
```

To version a few files of a file system that holds more, such as a module's stylesheet next to its templates, fingerprint only those with `assets.Only`; the manifest then knows, versions and serves those files alone, and `Fingerprint` fails when one is missing:

```go
manifest, err := assets.Fingerprint(module.Files, assets.Only("static/module.css"))
```

### Reloading while developing

Fingerprints are taken once. While developing with files on disk, `assets.Reload()` looks at the file system again on every use, cheaply, by the names, sizes and modification times of the files, and fingerprints it again when anything changed, was added or removed:

```go
manifest, err := assets.Fingerprint(os.DirFS("static"), assets.Reload())
im.WithAssets(manifest) // the import map follows the new versions too
```

It is safe for concurrent use. An embedded file system never changes, so leave it out in production; a change that keeps a file's size and modification time goes unnoticed. `manifest.Reloads()` reports whether a manifest was made with `Reload`.

A reloading file changes under pages that may be cached, by the browser or a server, and a page holding its old integrity makes the browser refuse the new content. So while the manifest reloads, `manifest.Stylesheet` and `manifest.TagIntegrity` leave the integrity out; `Integrity` still gives the current value. For an import map, render with `WithStableIntegrity` (see [Rendering](#rendering)).

## Building the map

| Method | Use |
|--------|-----|
| `WithFile(name, file)` | Maps a local file (below the assets directory or the file system of `FromFS`) to a name; its URL follows `WithURLFunc` and is versioned. A `.css` file becomes a stylesheet |
| `WithImport(name, url, integrity)` | Maps a name to a URL as it is, such as a route of your own or another host, with the module's Subresource Integrity for a module the manifest does not know, such as a library's own copy served at a route of yours; an empty integrity maps it without one |
| `ModulePreload(names...)` | Adds `<link rel="modulepreload">` for those imports |
| `URL(file)` | The URL of a local file, for `<script src>` and the like |
| `WithAssets(manifest)` | Versions the URLs of the fetched assets and makes their integrity known |
| `WithVersionedPaths(patterns...)` | Maps the plain URLs of local files to their versioned ones, for files scripts load by relative paths |
| `Script(file, attrs...)`, `Stylesheet(file, attrs...)` | A `<script>` or stylesheet tag for a local file, with its integrity under `WithIntegrity` (or `WithStableIntegrity`) |
| `Manifest()`, `ServeFile(w, r, file)` | The fingerprints of the local files, and serving one by name |

```go
im.WithImport("partial-sse", r.RoutePath("presence.client"), "").
	WithFile("app", "app/main.js").
	ModulePreload("htmx", "app")
```

`assets.Integrity(data)` or `manifest.Integrity(file)` gives the integrity value for `WithImport`.

## Rendering

`Render` returns the head HTML: stylesheet links in name order, the shim if one is set, the import map, and modulepreload links. `RenderWith` takes options for one call, `WithRenderOptions` sets them for every call:

```go
head, err := im.RenderWith(importmap.WithIntegrity())
```

`WithIntegrity` adds an `integrity` section to the import map and `integrity` attributes to the links, for files whose hash is known (`FromFS`, `WithAssets`) and imports mapped with `WithImport`. Attribute values are HTML-escaped; the JSON escapes `<`, `>` and `&`.

`WithStableIntegrity` is `WithIntegrity` for pages that may be cached: while the manifest reloads (`assets.Reload`), nothing carries integrity, neither the import map, its links, nor `Script` and `Stylesheet`; a manifest that does not reload renders as with `WithIntegrity`. It lets one setting serve development and production:

```go
im.WithRenderOptions(importmap.WithStableIntegrity())
```

In templates, `FuncMap` gives `{{importmap}}` (what `Render` produces), `{{importmap_nonce $nonce}}` (the same with a CSP nonce, see below), `{{importmap_url "app/main.js"}}`, and `importmap_script` and `importmap_style` for scripts and stylesheets a page loads directly rather than through the import map:

```go
tmpl := template.New("layout").Funcs(im.FuncMap())
```

```html
<head>
    {{importmap_script "theme.js"}}
    {{importmap_style "site.css"}}
    {{importmap}}
    {{importmap_script "app/main.js" "type" "module"}}
</head>
```

`importmap_script` and `importmap_style` (`Script` and `Stylesheet` in Go) write the file's versioned URL and, with `WithIntegrity` or `WithStableIntegrity` among the render options, its `integrity` with `crossorigin="anonymous"`, as the import map does for its entries. More attributes follow as name and value pairs, such as `"type" "module"` or `"data-api" $url`; an empty value writes the name alone (`"defer" ""`). A `crossorigin` attribute replaces the default one; `integrity` cannot be set this way, nor `src` on a script or `rel` and `href` on a stylesheet.

Configure the map before serving; rendering only reads it, so it is safe from many goroutines.

### Content-Security-Policy

The import map is an inline script, so a policy without `'unsafe-inline'` blocks it unless it allows that script by hash or by nonce. Everything else `Render` writes loads from a URL: the shim and the modulepreload links fall under `script-src`, the stylesheet links under `style-src`, and `'self'` covers them when the files are served by the site.

A hash suits pages that are cached, because it changes only when the map does. `ScriptHash` returns it as a source expression, quotes included:

```go
policy := "script-src 'self' " + im.ScriptHash()
// script-src 'self' 'sha256-…'
```

`ScriptHash` hashes the text between `<script type="importmap">` and `</script>`, made by the same code that renders it. Give it the options the page renders with: `WithIntegrity` changes the text, `WithNonce` does not; `WithRenderOptions` counts for both. Without options it is the hash of `{{importmap}}`. It is empty when there is no import map. While the manifest reloads (`assets.Reload`) the map follows the files, so take the hash and the page from the same moment, or use `RenderWithHash`, which returns both from one reading of the map:

```go
head, hash, err := im.RenderWithHash()
```

A nonce suits pages that are never cached: `WithNonce` adds `nonce="…"` to the shim, the import map and the modulepreload links. In templates, `{{importmap_nonce $nonce}}` is `{{importmap}}` with a nonce, and `importmap_script` takes one as an attribute (`"nonce" $nonce`). A nonce must differ for every response, so a cached page that carries one breaks; the hash stays valid with or without a nonce attribute.

## Fetching at start-up

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
				Name: "htmx", Version: "4.0.0",
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

The generated head contains a stylesheet link to `/assets/bootstrap/css/bootstrap.min.css` and an import map mapping `htmx` to `/assets/htmx/htmx.esm.min.js`. Import the module after the map:

```html
<script type="module">
    import htmx from "htmx";
    window.htmx = htmx;
</script>
```

An import map maps names to URLs; it does not execute modules. Use ESM files for module imports and ordinary script tags for libraries distributed as classic scripts.

`RootDir` controls filesystem reads and writes, while `AssetsDir` controls the asset directory and, without `WithURLFunc`, the public URL prefix. For example, `RootDir("/srv/my-app").AssetsDir("assets")` writes below `/srv/my-app/assets`, while URLs still begin with `/assets/`.

Cache files live below `.importmap/<package>/<version>/`; public assets live below `assets/<package>/`. `CacheOrFetch` maps packages from the cache, rebuilds missing assets from it without contacting the provider, and fetches only packages without a cache. A package without a version has no cache to find, so pin versions. To version the URLs and know the integrity of the published files, fingerprint them after `CacheOrFetch`:

```go
manifest, err := assets.Fingerprint(os.DirFS("assets"))
if err != nil {
	log.Fatal(err)
}
im.WithAssets(manifest)
```

Downloads use the supplied context, reject unsuccessful HTTP responses, and publish files only after a complete download. Retrying a failed fetch fills missing files without downloading complete files again. A published file that already exists is kept, so after changing a version remove the package's directory below `assets/` before fetching again.

## Providers and configuration

| Provider | Import path | Constructor |
|----------|-------------|-------------|
| cdnjs (default) | `client/cdnjs` | `cdnjs.New()` |
| jsDelivr | `client/jsdelivr` | `jsdelivr.New()` or `jsdelivr.NewESM()` |
| unpkg | `client/unpkg` | `unpkg.New()` |
| Skypack | `client/skypack` | `skypack.New()` |
| esm.sh | `client/esmsh` | `esmsh.New()` |
| Raw URL | `client/raw` | `raw.New(url)` |

Provider paths are relative to `github.com/donseba/go-importmap`. Implement `library.Provider` to add your own. Set a default with `WithProvider`, or override it through `library.Package.Provider`.

- `NewDefaults()` creates an import map with cdnjs, `assets` and `.importmap`; `New().WithDefaults()` is equivalent.
- `WithPackages` replaces the configured list with a copy; `WithPackage` appends one package.
- `library.Include.File` selects a provider file; `As` gives it an import name; `Raw` maps the name to a URL instead.
- `CacheDir`, `AssetsDir`, and `RootDir` set storage paths; `Clean` removes the cache and assets directories.
- `ShimPath` loads an ES module shim for browsers without import maps; none is loaded unless set. Vendor `es-module-shims` and pass its `URL`.
- `WithLogger` adds a `*slog.Logger`.
- `Fetch` contacts providers; `CacheOrFetch` reuses cached files when possible.
- `Render` produces the head HTML; `Imports` produces import-map JSON; `Marshal` includes imports, scopes, and styles.

## Raw URLs

For a single URL to download and cache locally, configure the package's `Provider` with `raw.New(url)` from `github.com/donseba/go-importmap/client/raw`. The raw provider uses the package name as the local filename; give that name a `.js` or `.css` extension so rendering can recognize the asset type. Public files still live below the package's asset directory.

## Upgrading

### From v1.4.0

Nothing exported was removed or changed. This change breaks compilation:

- `library.Package` has the new fields `License` and `Local`, so struct literals without field names, such as `library.Package{"htmx", "4.0.0", nil, nil}`, no longer compile. Name the fields.

These change behaviour.

Defaults:

- `NewDefaults` and `WithDefaults` no longer load the ES module shim from ga.jspm.io, so `Render` writes no shim `<script>` and `Shim()` returns `""`. Call `ShimPath("https://ga.jspm.io/npm:es-module-shims@2.0.10/dist/es-module-shims.js")` to keep it.

Rendering:

- `Render` and `Styles` write stylesheets in name order, without the invalid `as` attribute, with escaped `href` values; they wrote them in random order with `as="<name>"`. Tests that compare the exact HTML need the new output.
- `Render` escapes the shim's `src` too, which changes the HTML only for URLs with `&`, `<`, `>` or quotes.
- `Render` writes `Structure.Scopes` into the import map, so browsers apply them, also when the map holds only scopes. Before, it left the scopes out and wrote no map when there were no imports.

Fetching:

- `CacheOrFetch` fetches only the packages without a cache (it fetched every package for each missing one), applies `Raw` includes, and rebuilds missing asset files one by one, only the required ones.
- A package whose includes are all `Raw` no longer contacts its provider.
- A package without a version keeps the version the provider chose, so a second `CacheOrFetch` in the same process finds its cache.
- `WithPackages` copies the slice, so later changes to the caller's slice no longer reach the map.
- A `require` pattern that is not valid, such as `"dist/[a.js"`, makes `Fetch` and `CacheOrFetch` return an error before anything is fetched; the error was printed to standard output and the pattern skipped. `library.Includes.Check` reports it too.
- `CacheOrFetch` logs "asset not found, building from cache" once per missing file, with `package` and `file`; it logged "assets not found, building from cache" once per package. Update log filters that match the old message.
- With an `AssetsDir` that starts with `../`, such as `../public`, local asset URLs begin with `/public/`; they began with `/../public/`, which browsers resolve to the same address.

## License

Distributed under the MIT License. See [LICENSE](LICENSE).
