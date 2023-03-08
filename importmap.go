package importmap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	defaultPublishPath = path.Join("assets", "js")
	defaultCacheDir    = ".importmap"
)

type (
	Provider interface {
		Package(ctx context.Context, p *library.Package) (string, error)
	}

	ImportMap struct {
		Provider  Provider
		Packages  []library.Package
		Structure structure

		publish    bool
		publishDir string
		cacheDir   string
		rootDir    string
	}

	structure struct {
		Imports map[string]string            `json:"imports,omitempty"`
		Scopes  map[string]map[string]string `json:"scopes,omitempty"`
	}
)

func New(p Provider) *ImportMap {
	return &ImportMap{
		cacheDir:   defaultCacheDir,
		publishDir: defaultPublishPath,
		Provider:   p,
		Structure: structure{
			Imports: make(map[string]string),
			Scopes:  make(map[string]map[string]string),
		},
	}
}

func (im *ImportMap) SetPublish(b bool) {
	im.publish = b
}

func (im *ImportMap) SetRootDir(d string) {
	im.rootDir = d
}

func (im *ImportMap) SetPublishDir(d string) {
	im.publishDir = d
}

func (im *ImportMap) SetCashDir(d string) {
	im.cacheDir = d
}

func (im *ImportMap) Fetch(ctx context.Context) error {
	for _, pack := range im.Packages {
		name := pack.Name
		if pack.As != "" {
			name = pack.As
		}

		if pack.Raw != "" {
			im.Structure.Imports[name] = pack.Raw
			continue
		}

		if im.publish && im.publishExists(pack) {
			im.Structure.Imports[name] = im.publishPath(pack)

			continue
		}

		path, err := im.Provider.Package(ctx, &pack)
		if err != nil {
			return err
		}

		if im.publish {
			if !im.cacheExists(pack) {
				err = im.cacheMake(pack, path)
				if err != nil {
					return err
				}
			}

			err = im.publishMake(pack)
			if err != nil {
				return err
			}

			path = im.publishPath(pack)
		}

		im.Structure.Imports[name] = path
	}

	return nil
}

func (im *ImportMap) cachePath(p library.Package) string {
	return path.Join(im.cacheDir, p.Name, p.Version, p.FileName)
}

func (im *ImportMap) publishPath(p library.Package) string {
	return path.Join(im.publishDir, p.Name, p.FileName)
}

func (im *ImportMap) cacheExists(p library.Package) bool {
	fullPath := path.Join(im.rootDir, im.cachePath(p))

	if _, err := os.Stat(fullPath); errors.Is(err, os.ErrNotExist) {
		return false
	}

	return true
}

func (im *ImportMap) publishExists(p library.Package) bool {
	fullPath := path.Join(im.rootDir, im.publishPath(p))

	if _, err := os.Stat(fullPath); errors.Is(err, os.ErrNotExist) {
		return false
	}

	return true
}

func (im *ImportMap) cacheMake(p library.Package, url string) error {
	fullPath := path.Join(im.rootDir, im.cachePath(p))

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

func (im *ImportMap) publishMake(p library.Package) error {
	cachePath := path.Join(im.rootDir, im.cachePath(p))
	publishPath := path.Join(im.rootDir, im.publishPath(p))

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

func (im *ImportMap) Marshal() ([]byte, error) {
	return json.Marshal(im.Structure)
}

func (im *ImportMap) Render() (template.HTML, error) {
	funcMap := template.FuncMap{"join": func(m map[string]string) template.HTML {
		imports := make([]string, 0, len(m))
		for k, v := range m {
			imports = append(imports, fmt.Sprintf(`"%s": "%s"`, k, v))
		}
		return template.HTML(strings.Join(imports, ",\n"))
	}}

	t, err := template.New("").Funcs(funcMap).Parse(`<script async src="https://ga.jspm.io/npm:es-module-shims@1.7.0/dist/es-module-shims.js"></script>
<script type="importmap">
{
  "imports": { 
{{ join .Structure.Imports }}
  }
}
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
