# go-importmap
Golang importmap generator in early stages.

Usage:
```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/donseba/go-importmap/client/cdnjs"
	"github.com/donseba/go-importmap/library"
	"github.com/donseba/go-importmap
)

func main() {
	ctx := context.TODO()
	pr := cdnjs.New()
	im := importmap.New(pr)

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
		log.Fatal(err)
		return
	}

	tmpl, err := im.Render()
	if err != nil {
		log.Fatal(err)
		return
	}
	
	fmt.Println(tmpl)
}
```
Result: 
```html
<script async src="https://ga.jspm.io/npm:es-module-shims@1.7.0/dist/es-module-shims.js"></script>
<script type="importmap">
    {
        "imports": { 
            "htmx": "https://cdnjs.cloudflare.com/ajax/libs/htmx/1.8.6/htmx.min.js",
            "json-enc": "https://cdnjs.cloudflare.com/ajax/libs/htmx/1.8.6/ext/json-enc.min.js"
        }
    }
</script>
```

TODO: 
- local version of package to avoid CDN usage all the time
- update to newer version