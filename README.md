# go-importmap

go-importmap is a lightweight Go package for managing JavaScript and CSS dependencies. It fetches, caches, and generates import maps from popular CDNs, letting you focus on building your web applications.

## Supported Providers
 - **cdnjs**: Fetches library packages from the cdnjs CDN.
 - **jsdelivr**: Fetches library packages from the jsDelivr CDN.
 - **unpkg**: Fetches library packages from the unpkg CDN.
 - **skypack**: Fetches library packages from the skypack CDN.
 - **esm**: Fetches library packages from the esm.sh CDN.
 - **Raw**: Fetches files from a custom URL.

## Features
 - **Flexible Provider Interface**: Easily extendable to support multiple CDNs.
 - **Automatic Caching**: Downloads and caches library files to boost performance.
 - **Import Map Generation**: Automatically produces standards-compliant import maps.
 - **Customizable Directories**: Configure cache and asset directories to suit your project.
 - **Raw Imports**:Directly specify a URL to bypass the default provider.


## Installation

Install via Go modules:

```bash
go get github.com/donseba/go-importmap
```
###  Quick Example

Here's a quick example to get you started with ImportMap:

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/donseba/go-importmap"
	"github.com/donseba/go-importmap/library"
)

func main() {
	ctx := context.TODO()

	im := importmap.
		NewDefaults().
		WithPackages([]library.Package{
			{
				Name:    "htmx",
				Version: "1.9.10",
				Require: []library.Include{
					{ File: "htmx.min.js" },
					{ File: "/ext/json-enc.js", As: "json-enc" },
				},
			},
			{
				Name: "bootstrap",
				Require: []library.Include{
					{ File: "css/bootstrap.min.css" },
					{ File: "js/bootstrap.min.js", As: "bootstrap" },
				},
			},
		})

	if err := im.Fetch(ctx); err != nil {
		log.Fatal(err)
	}

	tmpl, err := im.Render()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(tmpl)
}
```
This code initializes ImportMap with default settings, fetches the specified libraries from your chosen provider, and generates an HTML snippet containing the necessary script and link tags.


Resulting in the following output:
```html
<link rel="stylesheet" href="/assets/css/bootstrap.min.css" as="bootstrap"/>
<script async src="https://ga.jspm.io/npm:es-module-shims@1.7.0"></script>
<script type="importmap">
    {
      "bootstrap": "/assets/js/bootstrap.min.js",
      "htmx": "/assets/htmx.min.js",
      "json-enc": "/assets/ext/json-enc.js"
    }
</script>
```
This above example initializes ImportMap with default settings and fetches the specified library packages from cdnjs. 
It then generates an import map with the required JavaScript and CSS files, including the ES module shim 
for compatibility with older browsers. 

Finally, it renders the HTML block with the necessary script tags for the libraries.

## Configuration

ImportMap offers several methods to customize its behavior according to your project's needs:

 - **WithDefaults()**: Initialize with sensible defaults for cache and asset directories.
 - **WithProvider(provider Provider)**: Set a custom provider for fetching library files.
 - **WithPackages(packages []library.Package)**: Add one or more library packages.
 - **WithPackage(package library.Package)**: Adds a single library package to the import map.
 - **AssetsDir(dir string)**: Sets the directory path for assets, default is `assets`.
 - **CacheDir(dir string)**: Sets the directory path for the cache, default is `.importmap`.
 - **RootDir(dir string)**: Sets the directory paths for assets, cache, and root directories, respectively.
 - **ShimPath(sp string)**:Specify the ES module shim URL.

## RAW Imports

it is possible to bypass the cdnjs by using the using the Raw provider:

```go
pr := cdnjs.New()
im := New().WithProvider(pr).WithLogger(slog.Default())

im.WithPackages([]library.Package{
    {
        Name:     "htmx",
        Version:  "2.0.4",
        Provider: raw.New("https://unpkg.com/browse/htmx.org@2.0.4/dist/htmx.min.js"),
    },
})
```
results in generating:
```json
    {"imports":{"htmx":"https://unpkg.com/browse/htmx.org@1.8.6/dist/htmx.min.js"}}
```

## Contributing

Contributions are welcome!
Whether it's bug reports, feature requests, or code contributions,
please feel free to reach out or submit a pull request.
## License

Distributed under the MIT License. See `LICENSE` for more information.