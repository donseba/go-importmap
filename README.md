# go-importmap

Golang importmap generator and super lightweight javascript library and asset manager.

In addition to generate the importmap section it can also cache external libraries and serve them from local storage.

For now only cdnjs has been implemented because it provides a great api to interact with. There is a `Raw` client as well that mimics the process and returns the Raw field of the package struct.

## Example

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/donseba/go-importmap"
	"github.com/donseba/go-importmap/client/cdnjs"
	"github.com/donseba/go-importmap/library"
)

func main() {
	ctx := context.TODO()
	pr := cdnjs.New()

	im := importmap.New(pr)
	im.SetUseAssets(true)

	im.Packages = []library.Package{
		{
			Name:    "htmx",
			Version: "1.8.5",
		},
		{
			Name:     "htmx",
			Version:  "1.8.4",
			As:       "json-enc",
			FileName: "ext/json-enc.min.js",
		},
		{
			Name:    "htmx-latest",
			Version: "1.8.6",
			Raw:     "https://unpkg.com/browse/htmx.org@1.8.6/dist/htmx.min.js",
		},
	}

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

Result in the following with `SetUseAssets` set to `false`:

```html
<script async src="https://ga.jspm.io/npm:es-module-shims@1.7.0/dist/es-module-shims.js"></script>
<script type="importmap">
{
  "imports": {
    "htmx": "https://cdnjs.cloudflare.com/ajax/libs/htmx/1.8.5/htmx.min.js",
    "htmx-latest": "https://unpkg.com/browse/htmx.org@1.8.6/dist/htmx.min.js",
    "json-enc": "https://cdnjs.cloudflare.com/ajax/libs/htmx/1.8.4/ext/json-enc.min.js"
  }
}
</script>
```

Result in the following with `SetUseAssets` set to `true`:

```html
<script async src="https://ga.jspm.io/npm:es-module-shims@1.7.0/dist/es-module-shims.js"></script>
<script type="importmap">
{
  "imports": {
    "htmx": "/assets/js/htmx/htmx.min.js",
    "htmx-latest": "/assets/js/htmx-latest/htmx.min.js",
    "json-enc": "/assets/js/htmx/ext/json-enc.min.js"
  }
}
</script>

```

Output will look like the following:

```
- .importmap
  - htmx
    - 1.8.4
      - ext
        - json-enc.min.js
    - 1.8.5
      - htmx.min.js
  - htmx-latest
    - 1.8.6 
      - htmx.min.js
- assets
  - js
    - htmx
      - ext
        - json-enc.min.js
      - htmx.min.js
    - htmx-latest
      - htmx.min.js
```

as you can see the `.importmap` contains the files per version and the assets are created without a version a version folder. This has been done by design so you don't have to update the snippet while doing an update.

## Variations

it is possible to bypass the cdnjs by using the `Raw` param on the package.

```go
	im.Packages = []library.Package{
		{
			Name: "htmx",
			Raw:  "https://some.url.to/repo/with.js",
		},
	}
```

This wil generate:
```json
	{"imports":{"htmx":"https://some.url.to/repo/with.js"}}
```