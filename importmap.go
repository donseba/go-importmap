package importmap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"net/http"
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
		Provider  Provider
		Packages  []library.Package
		Structure structure

		clean       bool
		assets      bool
		includeShim bool

		assetsDir  string
		assetsPath string
		cacheDir   string
		rootDir    string
		shimSrc    string
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

			if im.assets {
				if !im.cacheExists(pack) {
					err := im.cacheMake(pack, pack.Raw)
					if err != nil {
						return err
					}
				}

				err := im.publishMake(pack)
				if err != nil {
					return err
				}

				pathToRetrieve = im.getAssetsPath(pack)
			}

			im.Structure.Imports[name] = pathToRetrieve
			continue
		}

		pathToRetrieve, err := im.Provider.Package(ctx, &pack)
		if err != nil {
			return err
		}

		if im.assets && im.assetsExists(pack) {
			im.Structure.Imports[name] = im.getAssetsPath(pack)
			continue
		}

		if im.assets {
			if !im.cacheExists(pack) {
				err = im.cacheMake(pack, pathToRetrieve)
				if err != nil {
					return err
				}
			}

			err = im.publishMake(pack)
			if err != nil {
				return err
			}

			pathToRetrieve = im.getAssetsPath(pack)
		}

		im.Structure.Imports[name] = pathToRetrieve
	}

	return nil
}

// SetRootDir sets the root dir to use ase base for both cache and assets directory
func (im *ImportMap) SetRootDir(d string) {
	im.rootDir = d
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

// SetCacheDir sets the path used for cache
func (im *ImportMap) SetCacheDir(d string) {
	im.cacheDir = d
}

// getCacheDir sets the path used for cache
func (im *ImportMap) getCacheDir(p library.Package) string {
	version := "0.0.0"
	if p.Version != "" {
		version = p.Version
	}
	return path.Join(im.cacheDir, p.Name, version, p.FileName)
}

// SetClean sets the flag to do a clean run
func (im *ImportMap) SetClean(b bool) {
	im.clean = b
}

// UseAssets sets the flag to assets the files locally
func (im *ImportMap) UseAssets(b bool) {
	im.assets = b
}

// UseShim sets the flag to include the shim
func (im *ImportMap) UseShim(b bool) {
	im.includeShim = b
}

// GetShim gets the path to the shim
func (im *ImportMap) GetShim() string {
	return im.shimSrc
}

// SetShimSrc sets the source of the shim
func (im *ImportMap) SetShimSrc(s string) {
	im.shimSrc = s
}

// IncludeShim returns includeShim
func (im *ImportMap) IncludeShim() bool {
	return im.includeShim
}

// SetPublishDir sets assetsDir to the desired value
func (im *ImportMap) SetPublishDir(d string) {
	im.assetsDir = d
}

// getAssetsDir returns the assetsDir including package name and filename
func (im *ImportMap) getAssetsDir(p library.Package) string {
	return path.Join(im.assetsDir, p.Name, p.FileName)
}

// getAssetsPath returns the getAssetsPath including package name and filename
func (im *ImportMap) getAssetsPath(p library.Package) string {
	return path.Join(im.assetsPath, p.Name, p.FileName)
}

// cacheExists checks if the cached version of a package exists
func (im *ImportMap) cacheExists(p library.Package) bool {
	fullPath := path.Join(im.RootDir(), im.getCacheDir(p))

	if _, err := os.Stat(fullPath); errors.Is(err, os.ErrNotExist) {
		return false
	}

	return true
}

// assetsExists checks if the published version of a package exists
func (im *ImportMap) assetsExists(p library.Package) bool {
	fullPath := path.Join(im.rootDir, im.getAssetsDir(p))

	if _, err := os.Stat(fullPath); errors.Is(err, os.ErrNotExist) {
		return false
	}

	return true
}

// cacheMake makes a copy from the remote source to the local .importmap folder
func (im *ImportMap) cacheMake(p library.Package, url string) error {
	fullPath := path.Join(im.RootDir(), im.getCacheDir(p))

	err := os.MkdirAll(filepath.Dir(fullPath), os.ModeDir)
	if err != nil {
		return err
	}

	file, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer file.Close()

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return err
	}

	return nil
}

// publishMake makes a copy from the  local .importmap to the assets folder
func (im *ImportMap) publishMake(p library.Package) error {
	cachePath := path.Join(im.RootDir(), im.getCacheDir(p))
	publishPath := path.Join(im.RootDir(), im.getAssetsDir(p))

	err := os.MkdirAll(filepath.Dir(publishPath), os.ModeDir)
	if err != nil {
		return err
	}

	input, err := os.ReadFile(cachePath)
	if err != nil {
		return err
	}

	err = os.WriteFile(publishPath, input, 0644)
	if err != nil {
		return err
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

// HTML returns the structure in HTML.
func (im *ImportMap) HTML() (template.HTML, error) {
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
{{ .HTML }}
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
