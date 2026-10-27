package importmap

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"

	"github.com/donseba/go-importmap/assets"
	"github.com/donseba/go-importmap/library"
)

// LockFile is the name of the lock file Vendor writes next to the files.
const LockFile = "importmap.lock.json"

// lockVersion is the format of the lock file.
const lockVersion = 1

type (
	// Lock lists the vendored files: what Vendor wrote and FromFS reads.
	Lock struct {
		// LockfileVersion is the format of the lock file.
		LockfileVersion int `json:"lockfileVersion"`
		// Packages are in the order they were configured.
		Packages []LockedPackage `json:"packages"`
	}

	// LockedPackage is one vendored package.
	LockedPackage struct {
		Name     string `json:"name"`
		Version  string `json:"version,omitempty"`
		Provider string `json:"provider,omitempty"`
		// Local marks the files of a local package (library.Package.Local):
		// built by the application rather than downloaded. Their paths may
		// start with "../", so the lock checks files beside the vendor
		// directory; FromFS then needs a file system that holds them.
		Local bool `json:"local,omitempty"`
		// License is the vendored licence text, relative to the lock file,
		// or empty when none was found.
		License string       `json:"license,omitempty"`
		Files   []LockedFile `json:"files"`
	}

	// LockedFile is a vendored file, or a raw import left on its host.
	LockedFile struct {
		// Path is the file relative to the lock file, such as
		// "htmx/htmx.esm.min.js"; empty for a raw import.
		Path string `json:"path,omitempty"`
		// As is the name the file is imported or styled as; empty for a
		// file that is not mapped, such as a licence.
		As string `json:"as,omitempty"`
		// Source is the URL the file was downloaded from, or the URL of a
		// raw import; empty for a local file.
		Source string `json:"source,omitempty"`
		// Integrity is the Subresource Integrity of the file
		// ("sha384-...").
		Integrity string `json:"integrity,omitempty"`
	}
)

// ReadLock reads a lock file from fsys.
func ReadLock(fsys fs.FS, name string) (*Lock, error) {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, err
	}

	var lock Lock
	if err := json.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}

	if lock.LockfileVersion != lockVersion {
		return nil, fmt.Errorf("read %s: unknown lockfileVersion %d", name, lock.LockfileVersion)
	}

	return &lock, nil
}

// FromFS returns the import map of vendored packages, such as an embed.FS
// with the files Vendor wrote, without network or disk access. lock is the
// name of the lock file in fsys; the files it lists are relative to it.
//
// Every listed file must be in fsys with the integrity the lock gives. All
// files in fsys are fingerprinted, so URL versions any of them, WithFile can
// map them, and ServeFile serves them.
//
//	//go:embed vendor
//	var vendored embed.FS
//
//	im, err := importmap.FromFS(vendored, "vendor/importmap.lock.json")
func FromFS(fsys fs.FS, lock string) (*ImportMap, error) {
	l, err := ReadLock(fsys, lock)
	if err != nil {
		return nil, err
	}

	manifest, err := assets.Fingerprint(fsys)
	if err != nil {
		return nil, err
	}

	im := New().WithAssets(manifest)

	for _, pkg := range l.Packages {
		im.packages = append(im.packages, library.Package{
			Name:    pkg.Name,
			Version: pkg.Version,
			Local:   pkg.Local,
		})

		for _, file := range pkg.Files {
			if err := im.mapLockedFile(lock, pkg, file); err != nil {
				return nil, err
			}
		}
	}

	return im, nil
}

// mapLockedFile maps a file of pkg in the lock file lock: a raw import by
// its URL, a vendored or local file by its path, after checking it.
func (im *ImportMap) mapLockedFile(lock string, pkg LockedPackage, file LockedFile) error {
	if file.Path == "" {
		if file.As != "" && file.Source != "" {
			im.setURL(library.ExtractFileType(file.Source), file.As, file.Source)
		}

		return nil
	}

	name, err := checkLockedFile(im.manifest, lock, pkg, file)
	if err != nil {
		return err
	}

	if file.As != "" {
		im.setLocal(fileKind(name), file.As, name)
	}

	return nil
}

// checkLockedFile returns the name in the file system of a file of pkg in
// the lock file lock, after checking that the file is there with the
// integrity the lock gives.
func checkLockedFile(manifest *assets.Manifest, lock string, pkg LockedPackage, file LockedFile) (string, error) {
	name := path.Join(path.Dir(lock), file.Path)

	if !fs.ValidPath(name) {
		if pkg.Local {
			return "", fmt.Errorf(
				"%s: %s is outside the file system; read the lock from a file system that holds the local files",
				lock,
				file.Path,
			)
		}

		return "", fmt.Errorf("%s: invalid path %q", lock, file.Path)
	}

	if !manifest.Has(name) {
		return "", fmt.Errorf("%s: %s is missing", lock, name)
	}

	if file.Integrity != "" && manifest.Integrity(name) != file.Integrity {
		return "", fmt.Errorf("%s: %s does not match its integrity; vendor again", lock, name)
	}

	return name, nil
}
