package unpkg

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/donseba/go-importmap/library"
)

var (
	defaultApiBaseURL = "https://unpkg.com/%s@%s/?meta" // Package name + version
	defaultCdnBaseURL = "https://unpkg.com/%s@%s/"      // Base CDN URL
)

type (
	Client struct{}

	UnpkgMetaResponse struct {
		Type  string             `json:"type"`
		Path  string             `json:"path"`
		Files []UnpkgFileListing `json:"files"`
	}

	UnpkgFileListing struct {
		Path string `json:"path"`
		Type string `json:"type"`
	}
)

func New() *Client {
	return &Client{}
}

func (c *Client) FetchPackageFiles(ctx context.Context, name, version string) (library.Files, string, error) {
	// Resolve latest version if not specified
	if version == "" {
		versionResp, err := c.getLatestVersion(ctx, name)
		if err != nil {
			return nil, "", err
		}
		version = versionResp
	}

	// Get file listing from Unpkg's meta API
	metaUrl := fmt.Sprintf(defaultApiBaseURL, name, version)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metaUrl, nil)
	if err != nil {
		return nil, "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("unpkg API responded with status %d", resp.StatusCode)
	}

	var meta UnpkgMetaResponse
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return nil, "", err
	}

	// Build base CDN URL
	basePath := fmt.Sprintf(defaultCdnBaseURL, name, version)

	// Recursively collect all files
	var files library.Files
	c.walkFiles(meta.Files, basePath, &files)

	return files, version, nil
}

func (c *Client) walkFiles(listings []UnpkgFileListing, basePath string, files *library.Files) {
	for _, item := range listings {
		if item.Type == "directory" {
			// Recursively process nested directories
			subUrl := fmt.Sprintf("%s%s/?meta", basePath, item.Path)
			subReq, _ := http.NewRequest("GET", subUrl, nil)
			resp, err := http.DefaultClient.Do(subReq)
			if err != nil {
				continue
			}

			var subdir UnpkgMetaResponse
			err = json.NewDecoder(resp.Body).Decode(&subdir)
			if err != nil {
				continue
			}
			resp.Body.Close()

			c.walkFiles(subdir.Files, basePath, files)
		} else {
			// Add the file with proper typing to the list
			*files = append(*files, library.File{
				Type:      library.ExtractFileType(item.Path),
				Path:      fmt.Sprintf("%s%s", basePath, strings.TrimPrefix(item.Path, "/")),
				LocalPath: strings.TrimPrefix(item.Path, "/"),
			})
		}
	}
}

// Helper to get latest version from npm registry
func (c *Client) getLatestVersion(ctx context.Context, name string) (string, error) {
	registryUrl := fmt.Sprintf("https://registry.npmjs.org/%s", name)
	resp, err := http.Get(registryUrl)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var pkg struct {
		DistTags struct {
			Latest string `json:"latest"`
		} `json:"dist-tags"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&pkg); err != nil {
		return "", err
	}

	return pkg.DistTags.Latest, nil
}
