package raw

import (
	"context"
	"errors"

	"github.com/donseba/go-importmap/library"
)

// Provider is a raw URL provider.
// It implements the Provider interface.
type Provider struct {
	URL string
}

// New creates a new raw provider with the given URL.
func New(url string) *Provider {
	return &Provider{URL: url}
}

// FetchPackageFiles returns a single file with the raw URL.
// The version is passed through unchanged.
func (p *Provider) FetchPackageFiles(ctx context.Context, name, version string) (library.Files, string, error) {
	if p.URL == "" {
		return nil, "", errors.New("raw provider URL is empty")
	}

	file := library.File{
		Type:      library.ExtractFileType(p.URL),
		Path:      p.URL,
		LocalPath: name,
	}
	return library.Files{file}, version, nil
}
