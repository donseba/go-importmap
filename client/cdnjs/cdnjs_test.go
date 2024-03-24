package cdnjs

import (
	"context"
	"fmt"
	"testing"
)

func TestClient_Search(t *testing.T) {
	ctx := context.TODO()
	cs := New()

	var tests = []struct {
		name, version, filename string
		want                    string
	}{
		{"htmx", "1.9.10", "", "https://cdnjs.cloudflare.com/ajax/libs/htmx/1.9.10/htmx.min.js"},
		{"htmx", "1.8.6", "", "https://cdnjs.cloudflare.com/ajax/libs/htmx/1.8.6/htmx.min.js"},
		{"htmx", "1.8.0", "", "https://cdnjs.cloudflare.com/ajax/libs/htmx/1.8.0/htmx.min.js"},
		{"htmx", "1.8.6", "ext/json-enc.js", "https://cdnjs.cloudflare.com/ajax/libs/htmx/1.8.6/ext/json-enc.js"},
	}

	for _, tt := range tests {
		testName := fmt.Sprintf("%s,%s,%s", tt.name, tt.version, tt.filename)
		t.Run(testName, func(t *testing.T) {
			p, _, err := cs.FetchPackageFiles(ctx, tt.name, tt.version)
			if err != nil {
				t.Error(err)
			}

			var found bool
			for _, f := range p {
				if f.Path == tt.want {
					if tt.filename != "" && f.LocalPath != tt.filename {

					} else {
						found = true
					}

					break
				}
			}

			if !found {
				t.Errorf("got %s, want %s", p, tt.want)
			}
		})
	}
}
