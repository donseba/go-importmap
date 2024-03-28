package raw

import (
	"context"

	"github.com/sparkupine/importmap/library"
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
