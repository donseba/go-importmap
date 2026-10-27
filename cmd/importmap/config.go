package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/donseba/go-importmap"
	"github.com/donseba/go-importmap/client/cdnjs"
	"github.com/donseba/go-importmap/client/esmsh"
	"github.com/donseba/go-importmap/client/jsdelivr"
	"github.com/donseba/go-importmap/client/raw"
	"github.com/donseba/go-importmap/client/skypack"
	"github.com/donseba/go-importmap/client/unpkg"
	"github.com/donseba/go-importmap/library"
)

type (
	// config is the importmap.json file.
	config struct {
		Provider string          `json:"provider"`
		Packages []configPackage `json:"packages"`
	}

	// configPackage is a package of the configuration.
	configPackage struct {
		Name     string          `json:"name"`
		Version  string          `json:"version"`
		Provider string          `json:"provider"`
		URL      string          `json:"url"`
		As       string          `json:"as"`
		License  string          `json:"license"`
		Require  []configInclude `json:"require"`
		// Local lists files the application builds, by their paths
		// relative to the out directory, in require; see
		// library.Package.Local.
		Local bool `json:"local"`
	}

	// configInclude is a file a package requires (library.Include).
	configInclude struct {
		File string `json:"file"`
		Raw  string `json:"raw"`
		As   string `json:"as"`
	}
)

// readConfig reads the configuration file name, refusing unknown fields.
func readConfig(name string) (*config, error) {
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, err
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var c config
	if err := decoder.Decode(&c); err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}

	return &c, nil
}

// importMap is the import map with the configured packages.
func (c *config) importMap() (*importmap.ImportMap, error) {
	if len(c.Packages) == 0 {
		return nil, errors.New("the configuration lists no packages")
	}

	defaultProvider, err := provider(c.Provider)
	if err != nil {
		return nil, err
	}

	im := importmap.New().WithProvider(defaultProvider)

	for _, p := range c.Packages {
		if p.Name == "" {
			return nil, errors.New("a package has no name")
		}

		pkg := library.Package{
			Name:    p.Name,
			Version: p.Version,
			License: p.License,
		}

		for _, req := range p.Require {
			pkg.Require = append(pkg.Require, library.Include{File: req.File, Raw: req.Raw, As: req.As})
		}

		switch {
		case p.Local:
			if p.URL != "" || p.Provider != "" {
				return nil, fmt.Errorf("package %s: a local package has no url or provider", p.Name)
			}

			pkg.Local = true
		case p.URL != "":
			pkg.Provider = raw.New(p.URL)

			if p.As != "" {
				pkg.Require = append(pkg.Require, library.Include{File: p.Name, As: p.As})
			}
		case p.Provider != "":
			pkg.Provider, err = provider(p.Provider)
			if err != nil {
				return nil, fmt.Errorf("package %s: %w", p.Name, err)
			}
		}

		im.WithPackage(pkg)
	}

	return im, nil
}

// provider returns the provider of a name in the configuration.
func provider(name string) (library.Provider, error) {
	switch name {
	case "", "cdnjs":
		return cdnjs.New(), nil
	case "jsdelivr":
		return jsdelivr.New(), nil
	case "jsdelivr-esm":
		return jsdelivr.NewESM(), nil
	case "unpkg":
		return unpkg.New(), nil
	case "skypack":
		return skypack.New(), nil
	case "esmsh":
		return esmsh.New(), nil
	default:
		return nil, fmt.Errorf("unknown provider %q", name)
	}
}
