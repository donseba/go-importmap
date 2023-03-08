package cdnjs

import (
	"context"
	"fmt"
	"testing"

	"github.com/donseba/go-importmap/library"
)

func TestClient_Search(t *testing.T) {
	ctx := context.TODO()
	cs := New()

	var tests = []struct {
		name, version, filename string
		want                    string
	}{
		{"htmx", "1.8.6", "", "https://cdnjs.cloudflare.com/ajax/libs/htmx/1.8.6/htmx.min.js"},
		{"htmx", "9.9.9", "", "https://cdnjs.cloudflare.com/ajax/libs/htmx/1.8.6/htmx.min.js"},
		{"htmx", "1.8.0", "", "https://cdnjs.cloudflare.com/ajax/libs/htmx/1.8.0/htmx.min.js"},
		{"htmx", "1.8.6", "ext/json-enc.js", "https://cdnjs.cloudflare.com/ajax/libs/htmx/1.8.6/ext/json-enc.js"},
	}

	for _, tt := range tests {
		testName := fmt.Sprintf("%s,%s,%s", tt.name, tt.version, tt.filename)
		t.Run(testName, func(t *testing.T) {
			p, err := cs.Package(ctx, &library.Package{
				Name:     tt.name,
				Version:  tt.version,
				FileName: tt.filename,
			})
			if err != nil {
				t.Error(err)
			}

			if p != tt.want {
				t.Errorf("got %s, want %s", p, tt.want)
			}

			t.Log(p)
		})
	}
}
