package cdnjs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/sparkupine/importmap/library"
	"net/http"
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

func (c *Client) FetchPackageFiles(ctx context.Context, name, version string) (library.Files, string, error) {
	url := defaultApiBaseURL + name

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", errors.New(fmt.Sprintf("client api responded with code %d", resp.StatusCode))
	}

	var sr SearchResponse
	err = json.NewDecoder(resp.Body).Decode(&sr)
	if err != nil {
		return nil, "", err
	}

	var (
		useVersion = sr.Version
	)

	if version != "" && version != useVersion {
		for _, v := range sr.Versions {
			if version == v {
				useVersion = v
				break
			}
		}
	}

	basePath := c.cdnBaseURL + name + "/" + useVersion + "/"

	var files library.Files

	for _, assets := range sr.Assets {
		for _, v := range assets.Files {
			files = append(files, library.File{
				Type:      library.ExtractFileType(v),
				Path:      basePath + v,
				LocalPath: v,
			})
		}
	}

	if len(files) == 0 && sr.Filename != "" {
		files = append(files, library.File{
			Type:      library.ExtractFileType(sr.Filename),
			Path:      basePath + sr.Filename,
			LocalPath: sr.Filename,
		})
	}

	return files, useVersion, nil
}
