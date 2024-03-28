# go-importmap

ImportMap is a powerful Go package designed to simplify the management of JavaScript library dependencies for modern web applications. It facilitates the fetching, caching, and generation of import maps, allowing for seamless integration of JavaScript and CSS libraries directly into your projects. By abstracting the complexity of handling external library dependencies, ImportMap enables developers to focus on building feature-rich web applications without worrying about the underlying details of dependency management.
Features

 - **Flexible Provider Interface**: Easily extendable to support different sources for fetching library packages, with a default implementation for cdnjs.
 - **Automatic Caching**: Libraries are fetched and cached locally to improve load times and reduce external requests.
 - **Import Map Generation**: Automatically generates import maps for included JavaScript and CSS files, adhering to the latest web standards.
 - **Customizable Directories**: Configurable cache and assets directories to fit the structure of your project.
 - **Shim Management**: Supports the inclusion of ES module shims for backward compatibility with non-module browsers.
 - **Dynamic Package Management**: Add individual or multiple library packages with optional versioning and dependency requirements.

## Getting Started
### Installation

To use ImportMap in your Go project, install it as a module:

```bash
go get github.com/sparkupine/importmap
```
###  Usage Example

Here's a quick example to get you started with ImportMap:

```go
package main

import (
    "context"
    "fmt"
    "log"
    "log/slog"

    "github.com/sparkupine/importmap"
    "github.com/sparkupine/importmap/library"
)

func main() {
    ctx := context.TODO()

    im := importmap.
        NewDefaults().
        WithLogger(slog.Default()).
        ShimPath("https://ga.jspm.io/npm:es-module-shims@1.7.0")

    im.WithPackages([]library.Package{
        {
            Name:    "htmx",
            Version: "1.8.5", // locking a specific version
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
                    As:   "bootstrap",
                },
                {
                    File: "js/bootstrap.min.js",
                    As:   "bootstrap",
                },
            },
        },
    })

    // retrieve all libraries
    err := im.Fetch(ctx)
    if err != nil {
        log.Fatal(err)
        return
    }

    // render the html block including script tags.
    tmpl, err := im.Render()
    if err != nil {
        log.Fatal(err)
        return
    }

    fmt.Println(tmpl)
}
```
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

 - **WithDefaults()**: Initializes ImportMap with default settings for cache and assets directories, and the ES module shim.
 - **WithProvider(provider Provider)**: Sets a custom provider for fetching library files.
 - **WithPackages(packages []library.Package)**: Sets multiple library packages to the import map.
 - **WithPackage(package library.Package)**: Adds a single library package to the import map.
 - **AssetsDir(dir string)**: Sets the directory path for assets, default is `assets`.
 - **CacheDir(dir string)**: Sets the directory path for the cache, default is `.importmap`.
 - **RootDir(dir string)**: Sets the directory paths for assets, cache, and root directories, respectively.
 - **ShimPath(sp string)**: Sets the path to the ES module shim, default is `https://ga.jspm.io/npm:es-module-shims@1.7.0/dist/es-module-shims.js`.


## file structure

```
- .importmap
  - bootstrap
    - 5.1.3
      - css
        - bootstrap.css
        - bootstrap.min.css
        - bootstrap-grid.css
        - bootstrap-grid.min.css
      - js
        - bootstrap.js
        - bootstrap.bundle.js 
        - bootstrap.min.js
  - htmx
    - 1.8.5
        - htmx.min.js
        - ext
          - json-enc.js
          - ajax-header.js
          - apline-morphd.js
          - ... etc
- assets
    - bootstrap
      -5.1.3
        - css
          - bootstrap.min.css
        - js
          - bootstrap.min.js
    - htmx
        - 1.8.5
            - htmx.min.js
            - ext
        - json-enc.js
```

As you can see the `.importmap` contains all the files fetched from the cdnjs, while the `assets` contains the files that are used in the importmap.

## RAW Imports

it is possible to bypass the cdnjs by using the `Raw` param on the package.

```go
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
```

This wil generate:
```json
    {"imports":{"htmx":"https://unpkg.com/browse/htmx.org@1.8.6/dist/htmx.min.js"}}
```

## Contributing

Contributions are welcome!
Whether it's bug reports, feature requests, or code contributions,
please feel free to reach out or submit a pull request.
## License

Distributed under the MIT License. See `LICENSE` for more information.