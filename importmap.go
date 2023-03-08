package importmap

import (
	"bytes"
	"context"
	"encoding/json"
	"html/template"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/donseba/go-importmap/library"
)

var (
	defaultAssetsDir  = path.Join("assets", "js")
	defaultAssetsPath = "/" + strings.Join([]string{"assets", "js"}, "/")
	defaultCacheDir   = ".importmap"
	defaultShimSrc    = "https://ga.jspm.io/npm:es-module-shims@1.7.0/dist/es-module-shims.js"
)

type (
	Provider interface {
		Package(ctx context.Context, p *library.Package) (string, error)
	}

	ImportMap struct {
		Provider    Provider          // the js library provider
		Packages    []library.Package // the library packages we want to include
		Structure   structure         // the output structure
		clean       bool              // whether to clean cache and assets
		useAssets   bool              // use local assets or not
		includeShim bool              // include shim to support older browsers
		assetsDir   string            // assets directory
		assetsPath  string            // path to assets in the URL
		cacheDir    string            // cache directory
		rootDir     string            // application root directory
		shimSrc     string            // shim source in case the default one does not meet requirements.
	}

	structure struct {
		Imports map[string]string            `json:"imports,omitempty"`
		Scopes  map[string]map[string]string `json:"scopes,omitempty"`
	}
)

// New returns a new instance of the ImportMap
func New(p Provider) *ImportMap {
	return &ImportMap{
		cacheDir:    defaultCacheDir,
		assetsDir:   defaultAssetsDir,
		assetsPath:  defaultAssetsPath,
		shimSrc:     defaultShimSrc,
		includeShim: true,
		clean:       false,
		useAssets:   false,
		Provider:    p,
		Structure: structure{
			Imports: make(map[string]string),
			Scopes:  make(map[string]map[string]string),
		},
	}
}

// Fetch retrieves all packages locally or remotely
func (im *ImportMap) Fetch(ctx context.Context) error {
	if im.clean {
		err := os.RemoveAll(path.Join(im.RootDir(), im.cacheDir))
		if err != nil {
			return err
		}
		err = os.RemoveAll(path.Join(im.RootDir(), im.assetsDir))
		if err != nil {
			return err
		}
	}

	for _, pack := range im.Packages {
		name := pack.Name
		if pack.As != "" {
			name = pack.As
		}

		if pack.Raw != "" {
			pathToRetrieve := pack.Raw

			if pack.FileName == "" {
				pack.FileName = path.Base(pack.Raw)
			}

			if im.useAssets {
				if !pack.HasCache(im.RootDir(), im.cacheDir) {
					err := pack.MakeCache(im.RootDir(), im.cacheDir, pack.Raw)
					if err != nil {
						return err
					}
				}

				err := pack.MakeAssets(im.RootDir(), im.cacheDir, im.assetsDir)
				if err != nil {
					return err
				}

				pathToRetrieve = pack.AssetsPath(im.assetsPath)
			}

			im.Structure.Imports[name] = pathToRetrieve
			continue
		}

		pathToRetrieve, err := im.Provider.Package(ctx, &pack)
		if err != nil {
			return err
		}

		if im.useAssets && pack.HasAssets(im.RootDir(), im.assetsPath) {
			im.Structure.Imports[name] = pack.AssetsPath(im.assetsPath)
			continue
		}

		if im.useAssets {
			if !pack.HasCache(im.RootDir(), im.cacheDir) {
				err = pack.MakeCache(im.RootDir(), im.cacheDir, pathToRetrieve)
				if err != nil {
					return err
				}
			}

			err = pack.MakeAssets(im.RootDir(), im.cacheDir, im.assetsDir)
			if err != nil {
				return err
			}

			pathToRetrieve = pack.AssetsPath(im.assetsPath)
		}

		im.Structure.Imports[name] = pathToRetrieve
	}

	return nil
}

// SetRootDir sets the root dir to use ase base for both cache and useAssets directory
func (im *ImportMap) SetRootDir(d string) {
	im.rootDir = d
}

// SetCacheDir sets the path used for cache
func (im *ImportMap) SetCacheDir(d string) {
	im.cacheDir = d
}

// SetAssetsDir sets assetsDir to the desired value
func (im *ImportMap) SetAssetsDir(d string) {
	im.assetsDir = d
}

// SetAssetsPath sets assetsPath to the desired value
func (im *ImportMap) SetAssetsPath(d string) {
	im.assetsPath = d
}

// SetClean sets the flag to do a clean run
func (im *ImportMap) SetClean(b bool) {
	im.clean = b
}

// SetIncludeShim sets the flag to include the shim
func (im *ImportMap) SetIncludeShim(b bool) {
	im.includeShim = b
}

// SetShimSrc sets the source of the shim
func (im *ImportMap) SetShimSrc(s string) {
	im.shimSrc = s
}

// SetUseAssets sets the flag to useAssets the files locally
func (im *ImportMap) SetUseAssets(b bool) {
	im.useAssets = b
}

// RootDir retrieves the root dir
func (im *ImportMap) RootDir() string {
	if im.rootDir == "" {
		ex, err := os.Executable()
		if err != nil {
			return ""
		}
		im.rootDir = filepath.Dir(ex)
	}

	return im.rootDir
}

// GetShim gets the path to the shim
func (im *ImportMap) GetShim() string {
	return im.shimSrc
}

// IncludeShim returns includeShim
func (im *ImportMap) IncludeShim() bool {
	return im.includeShim
}

// Marshal returns the Structure as JSON.
func (im *ImportMap) Marshal() ([]byte, error) {
	return json.Marshal(im.Structure)
}

// MarshalIndent returns the Structure as JSON in a pretty form.
func (im *ImportMap) MarshalIndent() ([]byte, error) {
	return json.MarshalIndent(im.Structure, "", "  ")
}

// Imports returns the structure in HTML.
func (im *ImportMap) Imports() (template.HTML, error) {
	b, err := json.MarshalIndent(im.Structure, "", "  ")
	if err != nil {
		return "", err
	}

	return template.HTML(b), nil
}

// Render returns an HTML snippet to use in a template
func (im *ImportMap) Render() (template.HTML, error) {
	t, err := template.New("").Parse(`{{ if .IncludeShim }}<script async src="{{ .GetShim }}"></script>{{ end }}
<script type="importmap">
{{ .Imports }}
</script>`)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	err = t.Execute(&buf, im)
	if err != nil {
		return "", err
	}

	return template.HTML(buf.String()), nil
}
