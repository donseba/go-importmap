package importmap

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/sparkupine/importmap/client/cdnjs"
	"github.com/sparkupine/importmap/library"
	"html/template"
	"log/slog"
	"os"
	"path"
)

var (
	defaultAssetsDir = "assets"
	defaultCacheDir  = ".importmap"
	defaultShimSrc   = "https://ga.jspm.io/npm:es-module-shims@1.7.0/dist/es-module-shims.js"
)

type (
	Provider interface {
		FetchPackageFiles(ctx context.Context, name, version string) (library.Files, string, error)
	}

	ImportMap struct {
		provider  Provider          // the js library provider
		packages  []library.Package // the library packages we want to include
		Structure structure         // the output structure

		rootDir   string
		assetsDir *string
		cacheDir  *string

		shim   string
		logger *slog.Logger
	}

	structure struct {
		Imports map[string]string            `json:"imports,omitempty"`
		Scopes  map[string]map[string]string `json:"scopes,omitempty"`
		Styles  map[string]string            `json:"styles,omitempty"`
	}
)

// New returns a new instance of the ImportMap
func New() *ImportMap {
	return &ImportMap{
		Structure: structure{
			Imports: make(map[string]string),
			Scopes:  make(map[string]map[string]string),
			Styles:  make(map[string]string),
		},
	}
}

func NewDefaults() *ImportMap {
	return New().WithDefaults()
}

func (im *ImportMap) WithDefaults() *ImportMap {
	im.CacheDir(defaultCacheDir)
	im.AssetsDir(defaultAssetsDir)
	im.ShimPath(defaultShimSrc)
	im.WithProvider(cdnjs.New())
	return im
}

func (im *ImportMap) WithProvider(p Provider) *ImportMap {
	im.provider = p
	return im
}

func (im *ImportMap) WithPackages(p []library.Package) *ImportMap {
	im.packages = p
	return im
}

func (im *ImportMap) WithPackage(p library.Package) *ImportMap {
	im.packages = append(im.packages, p)
	return im
}

func (im *ImportMap) WithLogger(logger *slog.Logger) *ImportMap {
	im.logger = logger
	return im
}

func (im *ImportMap) Clean() *ImportMap {
	cacheDir := defaultCacheDir
	if im.cacheDir != nil {
		cacheDir = *im.cacheDir
	}

	assetsDir := defaultAssetsDir
	if im.assetsDir != nil {
		assetsDir = *im.assetsDir
	}

	_ = os.RemoveAll(path.Join(im.rootDir, cacheDir))
	_ = os.RemoveAll(path.Join(im.rootDir, assetsDir))
	return im
}

func (im *ImportMap) CacheDir(dir string) *ImportMap {
	im.cacheDir = &dir
	return im
}

func (im *ImportMap) AssetsDir(dir string) *ImportMap {
	im.assetsDir = &dir
	return im
}

func (im *ImportMap) RootDir(dir string) *ImportMap {
	im.rootDir = dir
	return im
}

func (im *ImportMap) Shim() string {
	return im.shim
}

func (im *ImportMap) ShimPath(sp string) *ImportMap {
	im.shim = sp
	return im
}

func (im *ImportMap) Fetch(ctx context.Context) error {
	for _, pkg := range im.packages {
		if im.logger != nil {
			im.logger.InfoContext(ctx, "fetching assets", "package", pkg.Name)
		}
		allFiles, version, err := im.provider.FetchPackageFiles(ctx, pkg.Name, pkg.Version)
		if err != nil {
			return err
		}

		if pkg.Version == "" {
			pkg.Version = version
		}

		if im.cacheDir != nil && !pkg.HasCache(im.rootDir, *im.cacheDir) {
			if im.logger != nil {
				im.logger.InfoContext(ctx, "building cache", "package", pkg.Name, "version", pkg.Version)
			}

			for _, file := range allFiles {
				err = pkg.MakeCache(im.rootDir, *im.cacheDir, file.LocalPath, file.Path)
				if err != nil {
					return err
				}
			}
		}

		var cacheDir string
		if im.cacheDir != nil {
			cacheDir = *im.cacheDir
		}

		if im.logger != nil {
			im.logger.InfoContext(ctx, "building assets", "package", pkg.Name, "version", pkg.Version)
		}

		assetFiles := make(library.Includes, 0)

		for _, file := range allFiles {
			var as string
			if len(pkg.Require) > 0 {
				req := pkg.Require.Get(file.LocalPath)
				if req == nil {
					continue
				}

				as = req.Name()
			} else {
				as = file.LocalPath
			}

			if im.assetsDir != nil {
				if !pkg.HasAssetFile(im.rootDir, *im.assetsDir, file.LocalPath) {
					err = pkg.MakeAssets(im.rootDir, cacheDir, *im.assetsDir, file.LocalPath, file.Path)
					if err != nil {
						return err
					}
				}

				assetFiles = append(assetFiles, library.Include{
					File: path.Join(im.rootDir, pkg.AssetsDir(*im.assetsDir), file.LocalPath),
					As:   as,
				})
			} else {
				assetFiles = append(assetFiles, library.Include{
					File: file.Path,
					As:   as,
				})
			}
		}

		for _, file := range assetFiles {
			// check if it starts with a /, if not, add it
			if file.File[0] != '/' && file.File[0] != 'h' {
				file.File = "/" + file.File

			}

			switch library.ExtractFileType(file.File) {
			case library.FileTypeCSS:
				im.Structure.Styles[file.As] = file.File
			case library.FileTypeJS:
				im.Structure.Imports[file.As] = file.File
			}
		}

		for _, req := range pkg.Require {
			if req.Raw != "" {
				im.Structure.Imports[req.Name()] = req.Raw
			}
		}
	}

	return nil
}

// Marshal returns the Structure as JSON.
func (im *ImportMap) Marshal() ([]byte, error) {
	return json.Marshal(im.Structure)
}

// MarshalIndent returns the Structure as JSON in a pretty form.
func (im *ImportMap) MarshalIndent() ([]byte, error) {
	return json.MarshalIndent(im.Structure, "", "  ")
}

// Imports return the structure in JSON/HTML.
func (im *ImportMap) Imports() (template.HTML, error) {
	var in = struct {
		Imports map[string]string `json:"imports"`
	}{
		Imports: im.Structure.Imports,
	}

	b, err := json.Marshal(in)
	if err != nil {
		return "", err
	}

	return template.HTML(b), nil
}

// ImportsIndent return the structure in JSON/HTML.
func (im *ImportMap) ImportsIndent() (template.HTML, error) {
	var in = struct {
		Imports map[string]string `json:"imports"`
	}{
		Imports: im.Structure.Imports,
	}

	b, err := json.MarshalIndent(in, "", "  ")
	if err != nil {
		return "", err
	}

	return template.HTML(b), nil
}

func (im *ImportMap) Scopes() (template.HTML, error) {
	b, err := json.MarshalIndent(im.Structure.Scopes, "", "  ")
	if err != nil {
		return "", err
	}

	return template.HTML(b), nil
}

func (im *ImportMap) Styles() (template.HTML, error) {
	if im.Structure.Styles == nil {
		return "", nil
	}

	var out string
	for k, v := range im.Structure.Styles {
		out += fmt.Sprintf(`<link rel="stylesheet" href="%s" as="%s">`, v, k)
	}

	return template.HTML(out), nil
}

// Render returns an HTML snippet to use in a template
func (im *ImportMap) Render() (template.HTML, error) {
	var out template.HTML

	for k, v := range im.Structure.Styles {
		out += template.HTML(fmt.Sprintf(`<link rel="stylesheet" href="%s" as="%s"/>
`, v, k))
	}

	if im.shim != "" {
		out += template.HTML(fmt.Sprintf(`<script async src="%s"></script>
`, im.shim))
	}

	if len(im.Structure.Imports) > 0 {
		out += `<script type="importmap">
`

		data := struct {
			Imports map[string]string `json:"imports"`
		}{
			Imports: im.Structure.Imports,
		}

		b, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return "", err
		}

		out += template.HTML(b)
		out += `
</script>`
	}

	return out, nil
}
