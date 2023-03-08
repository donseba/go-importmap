package cdnjs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/donseba/go-importmap/library"
)

var (
	defaultApiBaseURL = "https://api.cdnjs.com/libraries/"
	defaultCdnBaseURL = "https://cdnjs.cloudflare.com/ajax/libs/"
)

type (
	Client struct {
		apiBaseURL string
		cdnBaseURL string
	}

	SearchResponse struct {
		Name     string   `json:"name"`
		Latest   string   `json:"latest"`
		Filename string   `json:"filename"`
		Version  string   `json:"version"`
		Versions []string `json:"versions"`
		Assets   []Assets `json:"assets"`
	}

	Assets struct {
		Version string   `json:"version"`
		Files   []string `json:"files"`
	}
)

func New() *Client {
	return &Client{
		apiBaseURL: defaultApiBaseURL,
		cdnBaseURL: defaultCdnBaseURL,
	}
}

func (c *Client) Package(ctx context.Context, p *library.Package) (string, error) {
	url := defaultApiBaseURL + p.Name

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", errors.New(fmt.Sprintf("client api responded with code %d", resp.StatusCode))
	}

	var sr SearchResponse
	err = json.NewDecoder(resp.Body).Decode(&sr)
	if err != nil {
		return "", err
	}

	var (
		useVersion  = sr.Version
		useFilename = sr.Filename
		path        = sr.Latest
	)

	if p.Version != "" && p.Version != useVersion {
		for _, v := range sr.Versions {
			if p.Version == v {
				useVersion = v
				break
			}
		}
	}

	if p.FileName != "" && p.FileName != useFilename {
		for _, assets := range sr.Assets {
			for _, v := range assets.Files {
				if p.FileName == v {
					useFilename = v
				}
			}
		}
	}

	p.FileName = useFilename

	path = strings.Replace(path, sr.Version, useVersion, 1)
	path = strings.Replace(path, sr.Filename, useFilename, 1)

	return path, nil
}
