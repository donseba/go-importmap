# go-importmap
Golang importmap generator. 

disclaimer : There is still plenty of room for optimization. and the API might change during the early stages of development

Usage:
```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/donseba/go-importmap"
	"github.com/donseba/go-importmap/client/cdnjs"
	"github.com/donseba/go-importmap/library"
)

func main() {
	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	exPath := filepath.Dir(ex)

	ctx := context.TODO()
	pr := cdnjs.New()

	im := importmap.New(pr)
	im.SetPublish(true)
	im.SetRootDir(exPath)

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
	}

	err = im.Fetch(ctx)
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
Result in the following `without` publish set to true:
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

Result in the following `with` publish set to true: 
```html
<script async src="https://ga.jspm.io/npm:es-module-shims@1.7.0/dist/es-module-shims.js"></script>
<script type="importmap">
    {
        "imports": { 
            "htmx": "assets/js/htmx/htmx.min.js",
            "json-enc": "assets/js/htmx/ext/json-enc.min.js"
        }
    }
</script>
```

Files generated will look like the following: 

```
- .importmap
  - htmx
    - 1.8.4
      - ext
        - json-enc.min.js
    - 1.8.5
      - htmx.min.js
- assets
  - js
    - htmx
      - ext
        - json-enc.min.js
      - htmx.min.js
```
as you can see the `.importmap` contains the files per version and we create the assets without a version this will allow you to `update` the file without having to update the snippet. 