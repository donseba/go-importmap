package library

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

type Provider interface {
	FetchPackageFiles(ctx context.Context, name, version string) (Files, string, error)
}

type Includes []Include

type Include struct {
	File string
	Raw  string
	As   string
}

func (I Includes) Get(s string) *Include {
	for _, i := range I {
		// Compile the pattern, assuming 'File' is a valid regex pattern
		pattern := "^" + strings.Trim(i.File, `/`) + "$"    // Ensure the pattern matches the entire string
		pattern = strings.Replace(pattern, "**/", "**", -1) // Ensure the pattern matches the entire string
		pattern = strings.Replace(pattern, ".", "\\.", -1)  // Ensure the pattern matches the entire string
		pattern = strings.Replace(pattern, "**", ".*", -1)  // Ensure the pattern matches the entire string

		re, err := regexp.Compile(pattern)
		if err != nil {
			fmt.Println(err)
			// Handle the error (e.g., log or panic depending on your error handling strategy)
			continue // For this example, we'll just skip this iteration
		}

		if re.MatchString(s) {
			return &i
		}
	}

	return nil
}

func (I Include) Name() string {
	if I.As != "" {
		return I.As
	}

	as := strings.Split(path.Base(I.File), ".")
	if len(as) > 0 {
		as = strings.Split(as[0], "*")
		if len(as) > 0 {
			return as[len(as)-1]
		}
		return as[0]
	}

	return I.File
}

type Package struct {
	Name     string
	Version  string
	Provider Provider
	Require  Includes // Patterns to specify which files to include
}

// CacheDir returns the cache dir for the current package, we will store all files in here
func (p *Package) CacheDir(cacheDir string) string {
	version := "latest"
	if p.Version != "" {
		version = p.Version
	}
	return path.Join(cacheDir, p.Name, version)
}

// HasCache checks if the package has cache on disk
func (p *Package) HasCache(rootDir string, cacheDir string) bool {
	fullPath := path.Join(rootDir, p.CacheDir(cacheDir))

	if _, err := os.Stat(fullPath); errors.Is(err, os.ErrNotExist) {
		return false
	}

	return true
}

// MakeCache retrieves the file from the remote server and stores it locally
func (p *Package) MakeCache(rootDir string, cacheDir string, filePath string, src string) error {
	fullPath := path.Join(rootDir, p.CacheDir(cacheDir), filePath)

	err := os.MkdirAll(filepath.Dir(fullPath), os.FileMode(0755))
	if err != nil {
		return err
	}

	file, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer file.Close()

	resp, err := http.Get(src)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return err
	}

	return nil
}

// AssetsDir returns the assets dir for the current package, we will store all files in here
func (p *Package) AssetsDir(assets string) string {
	return path.Join(assets, p.Name)
}

// HasAssets checks if the package has assets on disk
func (p *Package) HasAssets(rootDir string, assetsDir string) bool {
	fullPath := path.Join(rootDir, p.AssetsDir(assetsDir))

	if _, err := os.Stat(fullPath); errors.Is(err, os.ErrNotExist) {
		return false
	}

	return true
}

func (p *Package) Assets(assetsDir string, filePath string) (Files, string, error) {
	// If filePath is empty, list all files in the package's asset directory
	baseDir := p.AssetsDir(assetsDir)
	var fullPath string
	if filePath == "" {
		fullPath = path.Join(baseDir)
	} else {
		// Remove leading slash if present
		filePath = strings.TrimPrefix(filePath, "/")
		fullPath = path.Join(baseDir, filePath)
	}

	if _, err := os.Stat(fullPath); errors.Is(err, os.ErrNotExist) {
		return nil, "", fmt.Errorf("asset file %s does not exist", fullPath)
	}

	// Use a recursive helper function to traverse directories
	return p.getFilesRecursively(fullPath, baseDir, filePath)
}

// getFilesRecursively will scan a directory and all its subdirectories for files
func (p *Package) getFilesRecursively(fullPath, baseDir, relativePath string) (Files, string, error) {
	files, err := os.ReadDir(fullPath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read directory %s: %w", fullPath, err)
	}

	var assetFiles Files
	for _, file := range files {
		// Create the current file/directory path
		currentRelativePath := path.Join(relativePath, file.Name())
		currentFullPath := path.Join(fullPath, file.Name())

		if file.IsDir() {
			// Recursively process subdirectory
			subFiles, _, err := p.getFilesRecursively(currentFullPath, baseDir, currentRelativePath)
			if err != nil {
				return nil, "", err
			}
			// Add files from subdirectory to our list
			assetFiles = append(assetFiles, subFiles...)
		} else {
			// It's a file, add it to our list
			assetPath := path.Join(baseDir, currentRelativePath)

			// Ensure assetPath has a leading slash for URL usage
			if !strings.HasPrefix(assetPath, "/") {
				assetPath = "/" + assetPath
			}

			assetFiles = append(assetFiles, File{
				Path:      assetPath,
				LocalPath: currentRelativePath,
				Type:      ExtractFileType(file.Name()),
			})
		}
	}

	return assetFiles, baseDir, nil
}

func (p *Package) HasAssetFile(rootDir string, assetsDir string, filePath string) bool {
	fullPath := path.Join(rootDir, p.AssetsDir(assetsDir), filePath)

	if _, err := os.Stat(fullPath); errors.Is(err, os.ErrNotExist) {
		return false
	}

	return true
}

// MakeAssets copies the cache files to the asset path without the version
func (p *Package) MakeAssets(rootDir string, cacheDir string, assetsDir string, filePath string, src string) error {
	fullPath := path.Join(rootDir, p.AssetsDir(assetsDir), filePath)

	err := os.MkdirAll(filepath.Dir(fullPath), os.FileMode(0755))
	if err != nil {
		return err
	}

	file, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer file.Close()

	if cacheDir != "" && p.HasCache(rootDir, cacheDir) {
		cachePath := path.Join(rootDir, p.CacheDir(cacheDir), filePath)
		cacheFile, err := os.Open(cachePath)
		if err != nil {
			return err
		}

		_, err = io.Copy(file, cacheFile)
		if err != nil {
			return err
		}

		return nil
	}

	resp, err := http.Get(src)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return err
	}

	return nil
}
