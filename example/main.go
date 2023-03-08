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
	im.SetUseAssets(true)
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
		{
			Name:    "htmx-latest",
			Version: "1.8.6",
			Raw:     "https://unpkg.com/browse/htmx.org@1.8.6/dist/htmx.min.js",
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
