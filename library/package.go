package library

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
)

type Package struct {
	Name     string
	Version  string
	As       string
	FileName string
	Raw      string
}

// AssetsDir returns the assets dir for the current package
func (p *Package) AssetsDir(assetsDir string) string {
	return path.Join(assetsDir, p.Name, p.FileName)
}

// AssetsPath returns the assets path for the current package
func (p *Package) AssetsPath(assetsPath string) string {
	return path.Join(assetsPath, p.Name, p.FileName)
}

// CacheDir returns the cache dir for the current package
func (p *Package) CacheDir(cacheDir string) string {
	version := "0.0.0"
	if p.Version != "" {
		version = p.Version
	}
	return path.Join(cacheDir, p.Name, version, p.FileName)
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
func (p *Package) MakeCache(rootDir string, cacheDir string, src string) error {
	fullPath := path.Join(rootDir, p.CacheDir(cacheDir))

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

// HasAssets checks if the package has assets on disk
func (p *Package) HasAssets(rootDir string, assetsDir string) bool {
	fullPath := path.Join(rootDir, p.AssetsDir(assetsDir))

	if _, err := os.Stat(fullPath); errors.Is(err, os.ErrNotExist) {
		return false
	}

	return true
}

// MakeAssets copies the cache files to the asset path without the version
func (p *Package) MakeAssets(rootDir string, cacheDir string, assetsDir string) error {
	cachePath := path.Join(rootDir, p.CacheDir(cacheDir))
	assetsPath := path.Join(rootDir, p.AssetsDir(assetsDir))

	err := os.MkdirAll(filepath.Dir(assetsPath), os.FileMode(0755))
	if err != nil {
		return err
	}

	input, err := os.ReadFile(cachePath)
	if err != nil {
		return err
	}

	err = os.WriteFile(assetsPath, input, 0644)
	if err != nil {
		return err
	}

	return nil
}
