package cdnjs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

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
		Files    []string `json:"files"`
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
	apiBaseURL := c.apiBaseURL
	if apiBaseURL == "" {
		apiBaseURL = defaultApiBaseURL
	}
	cdnBaseURL := c.cdnBaseURL
	if cdnBaseURL == "" {
		cdnBaseURL = defaultCdnBaseURL
	}
	url := apiBaseURL + name
	if version != "" {
		url += "/" + version
	}

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
		return nil, "", fmt.Errorf("client api responded with code %d", resp.StatusCode)
	}

	var sr SearchResponse
	err = json.NewDecoder(resp.Body).Decode(&sr)
	if err != nil {
		return nil, "", err
	}

	useVersion := sr.Version
	if version != "" && useVersion != version {
		return nil, "", fmt.Errorf("cdnjs returned version %q for requested version %q", useVersion, version)
	}

	basePath := cdnBaseURL + name + "/" + useVersion + "/"

	var files library.Files

	fileNames := sr.Files
	if version == "" && len(sr.Assets) > 0 {
		fileNames = sr.Assets[0].Files
	}
	for _, v := range fileNames {
		files = append(files, library.File{
			Type:      library.ExtractFileType(v),
			Path:      basePath + v,
			LocalPath: v,
		})
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
