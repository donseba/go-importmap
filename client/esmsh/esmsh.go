package esmsh

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"

	"github.com/donseba/go-importmap/library"
)

var (
	defaultApiBaseURL = "https://esm.sh/"
)

// Client holds configuration for esm.sh requests.
type Client struct {
	apiBaseURL string
}

// New creates a new esm.sh client.
func New() *Client {
	return &Client{
		apiBaseURL: defaultApiBaseURL,
	}
}

// FetchPackageFiles retrieves package metadata from esm.sh.
// It calls the ?meta endpoint, then parses the returned JavaScript snippet to extract the version
// and the main export file URL. It returns a single file in the library.Files slice.
func (c *Client) FetchPackageFiles(ctx context.Context, name, version string) (library.Files, string, error) {
	// If a version is provided, use it; otherwise, let esm.sh resolve the latest version.
	var pkgID string
	if version != "" {
		pkgID = fmt.Sprintf("%s@%s", name, version)
	} else {
		pkgID = name
	}

	// esm.sh meta endpoint returns a JavaScript snippet
	metaURL := fmt.Sprintf("%s%s?meta", c.apiBaseURL, pkgID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metaURL, nil)
	if err != nil {
		return nil, "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("esm.sh meta API responded with code %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	text := string(body)

	// Extract version from the leading comment.
	// Expected format: "/* esm.sh - bootstrap@5.3.3 */"
	reVersion := regexp.MustCompile(`/\*\s*esm\.sh\s*-\s*` + regexp.QuoteMeta(name) + `@([^\s*]+)`)
	matches := reVersion.FindStringSubmatch(text)
	if len(matches) < 2 {
		return nil, "", errors.New("failed to parse version from esm.sh meta output")
	}
	parsedVersion := matches[1]

	// Extract file URL from the export statement.
	// Expected export line: export * from "/bootstrap@5.3.3/es2022/bootstrap.mjs";
	reExport := regexp.MustCompile(`export\s+\*\s+from\s+["'](\/` + regexp.QuoteMeta(name) + `@[^"']+)["']`)
	matches = reExport.FindStringSubmatch(text)
	if len(matches) < 2 {
		return nil, "", errors.New("failed to parse export file URL from esm.sh meta output")
	}
	filePath := matches[1]

	// Construct the full file URL.
	// esm.sh URLs are absolute, so we prepend the API base URL (ensuring no double slash).
	fileURL := c.apiBaseURL[:len(c.apiBaseURL)-1] + filePath

	file := library.File{
		Type:      library.ExtractFileType(fileURL),
		Path:      fileURL,
		LocalPath: filePath,
	}
	files := library.Files{file}

	return files, parsedVersion, nil
}
