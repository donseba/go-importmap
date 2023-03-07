package raw

import (
	"context"

	"github.com/donseba/go-importmap/library"
)

type (
	Client struct{}
)

func New() *Client {
	return &Client{}
}

func (c *Client) Package(ctx context.Context, p library.Package) (string, error) {
	return p.Raw, nil
}
