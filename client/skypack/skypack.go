package skypack

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/donseba/go-importmap/library"
)

var (
	defaultPackageApiBaseURL = "https://api.skypack.dev/v1/package/"
	defaultBrowseApiBaseURL  = "https://api.skypack.dev/v1/browse/"
	defaultCdnBaseURL        = "https://cdn.skypack.dev/"
)

// Client holds configuration for Skypack requests.
type Client struct {
	packageApiBaseURL string
	browseApiBaseURL  string
	cdnBaseURL        string
}

// PackageResponse represents the response structure from /v1/package/{name}.
type PackageResponse struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// BrowseResponse represents the response structure from /v1/browse/{name}/{version}.
type BrowseResponse struct {
	Files []FileInfo `json:"files"`
}

// FileInfo represents individual file information in the browse response.
type FileInfo struct {
	Name   string  `json:"name"`
	SizeKB float64 `json:"sizeKB"`
	URL    string  `json:"url"`
}

// New creates a new Skypack client.
func New() *Client {
	return &Client{
		packageApiBaseURL: defaultPackageApiBaseURL,
		browseApiBaseURL:  defaultBrowseApiBaseURL,
		cdnBaseURL:        defaultCdnBaseURL,
	}
}

// FetchPackageFiles retrieves package metadata and file list from Skypack.
// It first calls the package endpoint to determine the package version (if not explicitly provided)
// and then calls the browse endpoint to retrieve the list of files.
func (c *Client) FetchPackageFiles(ctx context.Context, name, version string) (library.Files, string, error) {
	// Get package metadata from /v1/package/{name}
	packageURL := c.packageApiBaseURL + name
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, packageURL, nil)
	if err != nil {
		return nil, "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("Skypack package API responded with code %d", resp.StatusCode)
	}

	var pr PackageResponse
	if err = json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, "", err
	}

	// Determine which version to use.
	useVersion := pr.Version
	if version != "" && version != useVersion {
		useVersion = version
	}

	// Get file list from /v1/browse/{name}/{version}
	browseURL := fmt.Sprintf("%s%s/%s", c.browseApiBaseURL, name, useVersion)
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, browseURL, nil)
	if err != nil {
		return nil, "", err
	}

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("skypack browse API responded with code %d", resp.StatusCode)
	}

	var br BrowseResponse
	if err = json.NewDecoder(resp.Body).Decode(&br); err != nil {
		return nil, "", err
	}

	var files library.Files
	for _, fileInfo := range br.Files {
		// Construct a file entry using the URL returned in the response.
		files = append(files, library.File{
			Type:      library.ExtractFileType(fileInfo.Name),
			Path:      fileInfo.URL,
			LocalPath: fileInfo.Name,
		})
	}

	// Fallback: If no files were returned, use the package entry point.
	if len(files) == 0 {
		fallbackURL := c.cdnBaseURL + name + "@" + useVersion
		files = append(files, library.File{
			Type:      library.ExtractFileType(fallbackURL),
			Path:      fallbackURL,
			LocalPath: name,
		})
	}

	return files, useVersion, nil
}
