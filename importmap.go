package importmap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"strings"

	"github.com/donseba/go-importmap/library"
)

type (
	Provider interface {
		Package(ctx context.Context, p library.Package) (string, error)
	}

	ImportMap struct {
		Provider  Provider
		Packages  []library.Package
		Structure structure
	}

	structure struct {
		Imports map[string]string            `json:"imports,omitempty"`
		Scopes  map[string]map[string]string `json:"scopes,omitempty"`
	}
)

func New(p Provider) *ImportMap {
	return &ImportMap{
		Provider: p,
		Structure: structure{
			Imports: make(map[string]string),
			Scopes:  make(map[string]map[string]string),
		},
	}
}

func (im *ImportMap) Fetch(ctx context.Context) error {
	for _, pack := range im.Packages {
		path, err := im.Provider.Package(ctx, pack)
		if err != nil {
			return err
		}

		name := pack.Name
		if pack.As != "" {
			name = pack.As
		}

		im.Structure.Imports[name] = path
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
