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
