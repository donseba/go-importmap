package importmap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/donseba/go-importmap/assets"
	"github.com/donseba/go-importmap/library"
)

// maxVendorFile is the largest file Vendor downloads.
const maxVendorFile = 64 << 20

// licenseFile matches the usual names of a licence text.
var licenseFile = regexp.MustCompile(`(?i)^(licen[cs]e|copying)([.-].*)?$`)

// Vendor downloads the files of every package into dir, for an application
// to embed and load with FromFS, and writes the lock file (LockFile) there.
// It is meant for build time, for example through the importmap command:
//
//	//go:generate go run github.com/donseba/go-importmap/cmd/importmap vendor -config importmap.json -out assets/vendor
//
// A package's files go below dir/<package name>/: the files its Require
// patterns match (all files without patterns), and its licence: the file or
// URL of Package.License, or else a LICENSE, LICENCE or COPYING file of the
// package. Raw imports stay on their host. The lock lists each file with
// its source URL and SHA-384 integrity.
//
// Files whose content already matches the previous lock are not downloaded
// again, and files only the previous lock listed are removed. A previous
// lock that lists a downloaded file outside dir, such as "../x.js" after a
// hand edit, is refused before anything is downloaded or removed.
//
// A local package (library.Package.Local) is not downloaded: the lock
// records the integrity of the files the application built, so FromFS and
// importmap verify check them too. Those files are never removed.
func (im *ImportMap) Vendor(ctx context.Context, dir string) (*Lock, error) {
	v, err := openVendorDir(dir)
	if err != nil {
		return nil, err
	}

	lock, err := im.lockPackages(dir, func(pkg *library.Package) (LockedPackage, error) {
		if pkg.Local {
			return lockLocal(dir, pkg)
		}

		return im.vendorPackage(ctx, v, pkg)
	})
	if err != nil {
		return nil, err
	}

	v.removeDropped(lock)

	return lock, nil
}

// VendorLocal records the integrity of the local packages' files in the lock
// in dir again, without network access, for example after a build changed a
// bundle. Every other package keeps its entry of the lock as it is; it must
// have one, so run Vendor first.
func (im *ImportMap) VendorLocal(dir string) (*Lock, error) {
	previous, err := ReadLock(os.DirFS(dir), LockFile)
	if err != nil {
		return nil, err
	}

	vendored := make(map[string]LockedPackage)
	for _, pkg := range previous.Packages {
		vendored[pkg.Name] = pkg
	}

	return im.lockPackages(dir, func(pkg *library.Package) (LockedPackage, error) {
		if pkg.Local {
			return lockLocal(dir, pkg)
		}

		locked, ok := vendored[pkg.Name]
		if !ok || locked.Local {
			return locked, fmt.Errorf("not in %s; vendor the packages first", LockFile)
		}

		return locked, nil
	})
}

// lockPackages makes the lock of the configured packages, in their order,
// with the entry entry gives for each, and writes it to dir.
func (im *ImportMap) lockPackages(dir string, entry func(pkg *library.Package) (LockedPackage, error)) (*Lock, error) {
	if err := im.checkPatterns(); err != nil {
		return nil, err
	}

	lock := &Lock{LockfileVersion: lockVersion}

	for i := range im.packages {
		pkg := &im.packages[i]

		locked, err := entry(pkg)
		if err != nil {
			return nil, fmt.Errorf("vendor %s: %w", pkg.Name, err)
		}

		lock.Packages = append(lock.Packages, locked)
	}

	if err := writeLock(dir, lock); err != nil {
		return nil, err
	}

	return lock, nil
}

// vendorDir is the directory Vendor writes to, with the downloaded files
// its previous lock lists, by path.
type vendorDir struct {
	dir   string
	known map[string]LockedFile
}

// openVendorDir reads the downloaded files of the previous lock in dir;
// without a lock there are none. Vendor removes the files that drop out of
// the lock, so a path of a hand-edited lock that leads outside dir is an
// error.
func openVendorDir(dir string) (*vendorDir, error) {
	v := &vendorDir{dir: dir, known: make(map[string]LockedFile)}

	previous, err := ReadLock(os.DirFS(dir), LockFile)
	if errors.Is(err, fs.ErrNotExist) {
		return v, nil
	}

	if err != nil {
		return nil, err
	}

	for _, pkg := range previous.Packages {
		// Local files are the application's; they are not removed.
		if pkg.Local {
			continue
		}

		for _, file := range pkg.Files {
			if file.Path == "" {
				continue
			}

			if !vendoredPath(file.Path) {
				return nil, fmt.Errorf("%s lists %q outside the vendor directory", LockFile, file.Path)
			}

			v.known[file.Path] = file
		}
	}

	return v, nil
}

// removeDropped removes the files the previous lock listed that lock does
// not list.
func (v *vendorDir) removeDropped(lock *Lock) {
	current := make(map[string]bool)

	for _, pkg := range lock.Packages {
		for _, file := range pkg.Files {
			current[file.Path] = true
		}
	}

	for name := range v.known {
		if !current[name] {
			_ = os.Remove(filepath.Join(v.dir, filepath.FromSlash(name)))
		}
	}
}

// vendoredPath reports whether name, the path of a downloaded file in the
// lock, stays inside the vendor directory: slash-separated, relative and
// without "." or ".." elements or backslashes, as save writes them.
func vendoredPath(name string) bool {
	return fs.ValidPath(name) && name != "." && !strings.Contains(name, `\`)
}

// writeLock writes lock to dir.
func writeLock(dir string, lock *Lock) error {
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}

	return writeFileAtomic(filepath.Join(dir, LockFile), append(data, '\n'))
}

// lockLocal is the lock entry of a local package: the integrity of each
// file Require lists, by its path relative to dir, and of its licence.
func lockLocal(dir string, pkg *library.Package) (LockedPackage, error) {
	locked := LockedPackage{Name: pkg.Name, Version: pkg.Version, Local: true}

	add := func(file, as string) error {
		name := localFileName(file)
		if file == "" || path.IsAbs(name) || strings.Contains(name, "\\") {
			return fmt.Errorf("invalid local file %q: give a path relative to the vendor directory", file)
		}

		if hiddenPath(name) {
			return fmt.Errorf("local file %q: a name that starts with \".\" is not served", file)
		}

		if containsPath(locked.Files, name) {
			return nil
		}

		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil {
			return err
		}

		locked.Files = append(locked.Files, LockedFile{
			Path:      name,
			As:        as,
			Integrity: assets.Integrity(data),
		})

		return nil
	}

	for _, req := range pkg.Require {
		if req.Raw != "" {
			locked.Files = append(locked.Files, rawEntry(req))

			continue
		}

		if err := add(req.File, lockedName(req.As, req.File)); err != nil {
			return locked, err
		}
	}

	if pkg.License != "" {
		if err := add(pkg.License, ""); err != nil {
			return locked, fmt.Errorf("licence: %w", err)
		}

		locked.License = localFileName(pkg.License)
	}

	if len(locked.Files) == 0 {
		return locked, errors.New("a local package lists no files")
	}

	return locked, nil
}

// localFileName is the path of a local file as the lock records it:
// cleaned, without a leading "./".
func localFileName(file string) string {
	return path.Clean(strings.TrimPrefix(file, "./"))
}

// hiddenPath reports whether a part of name, a slash-separated path,
// starts with a dot, such as ".github/x.js" or "dist/.map". The assets
// package neither fingerprints nor serves such files, so the lock leaves
// them out: FromFS would report them missing.
func hiddenPath(name string) bool {
	for part := range strings.SplitSeq(name, "/") {
		if part != "." && part != ".." && strings.HasPrefix(part, ".") {
			return true
		}
	}

	return false
}

// lockedName is the name the lock records for file, which a package maps
// as as: as for a script or a stylesheet, which the map holds, and empty
// for any other file, such as a licence.
func lockedName(as, file string) string {
	if !mappable(library.ExtractFileType(file)) {
		return ""
	}

	return as
}

// rawEntry is the lock entry of a raw import, which stays on its host.
func rawEntry(req library.Include) LockedFile {
	return LockedFile{As: req.Name(), Source: req.Raw}
}

// vendorPackage downloads pkg into the vendor directory, unless all it
// requires are raw imports, and returns its lock entry, with the raw
// imports, which stay on their hosts.
func (im *ImportMap) vendorPackage(ctx context.Context, v *vendorDir, pkg *library.Package) (LockedPackage, error) {
	im.logInfo(ctx, "vendoring package", "package", pkg.Name, "version", pkg.Version)

	locked := LockedPackage{Name: pkg.Name, Version: pkg.Version}

	if !rawOnly(pkg) {
		var err error

		locked, err = im.downloadPackage(ctx, v, pkg)
		if err != nil {
			return locked, err
		}
	}

	for _, req := range pkg.Require {
		if req.Raw != "" {
			locked.Files = append(locked.Files, rawEntry(req))
		}
	}

	slices.SortStableFunc(locked.Files, func(a, b LockedFile) int {
		return strings.Compare(a.Path, b.Path)
	})

	return locked, nil
}

// downloadPackage downloads the files of pkg its Require patterns select,
// and its licence, into the vendor directory, and returns its lock entry.
func (im *ImportMap) downloadPackage(ctx context.Context, v *vendorDir, pkg *library.Package) (LockedPackage, error) {
	provider := im.providerFor(pkg)
	if provider == nil {
		return LockedPackage{}, errors.New("no provider")
	}

	files, err := fetchFiles(ctx, provider, pkg)
	if err != nil {
		return LockedPackage{}, err
	}

	locked := LockedPackage{
		Name:     pkg.Name,
		Version:  pkg.Version,
		Provider: providerName(provider),
	}

	for _, file := range files {
		if hiddenPath(file.LocalPath) {
			continue
		}

		as, ok := importName(pkg, file.LocalPath)
		if !ok {
			continue
		}

		lf, err := v.save(ctx, pkg.Name, file.LocalPath, file.Path)
		if err != nil {
			return locked, err
		}

		lf.As = lockedName(as, file.LocalPath)
		locked.Files = append(locked.Files, lf)
	}

	if len(locked.Files) == 0 {
		return locked, errors.New("no files match the package's Require patterns")
	}

	if err := v.saveLicense(ctx, pkg, files, &locked); err != nil {
		return locked, fmt.Errorf("licence: %w", err)
	}

	return locked, nil
}

// saveLicense downloads the licence of pkg, a file among files or the URL
// of Package.License, next to the package's files, unless it is one of
// them already, and records it in locked.
func (v *vendorDir) saveLicense(ctx context.Context, pkg *library.Package, files library.Files, locked *LockedPackage) error {
	license, source := licenseSource(pkg, files)
	if source == "" {
		return nil
	}

	locked.License = path.Join(pkg.Name, license)

	if containsPath(locked.Files, locked.License) {
		return nil
	}

	lf, err := v.save(ctx, pkg.Name, license, source)
	if err != nil {
		return err
	}

	locked.Files = append(locked.Files, lf)

	return nil
}

// save downloads src to <pkg>/<localPath> in the vendor directory, unless
// the file there already has the content the previous lock gives for the
// same source, and returns its lock entry.
func (v *vendorDir) save(ctx context.Context, pkg, localPath, src string) (LockedFile, error) {
	// The file must stay below the package's directory.
	name := path.Join(pkg, localPath)
	if !fs.ValidPath(pkg) || !fs.ValidPath(name) || !strings.HasPrefix(name, path.Clean(pkg)+"/") {
		return LockedFile{}, fmt.Errorf("invalid file path %q", localPath)
	}

	filename := filepath.Join(v.dir, filepath.FromSlash(name))
	file := LockedFile{Path: name, Source: src}

	if previous, ok := v.known[name]; ok && previous.Source == src {
		if data, err := os.ReadFile(filename); err == nil && assets.Integrity(data) == previous.Integrity {
			file.Integrity = previous.Integrity

			return file, nil
		}
	}

	data, err := download(ctx, src)
	if err != nil {
		return file, err
	}

	if err := writeFileAtomic(filename, data); err != nil {
		return file, err
	}

	file.Integrity = assets.Integrity(data)

	return file, nil
}

// licenseSource is the file name and URL of pkg's licence, if any.
func licenseSource(pkg *library.Package, files library.Files) (string, string) {
	if pkg.License != "" {
		if strings.Contains(pkg.License, "://") {
			name := path.Base(pkg.License)
			if !licenseFile.MatchString(name) {
				name = "LICENSE"
			}

			return name, pkg.License
		}

		for _, file := range files {
			if file.LocalPath == strings.TrimPrefix(pkg.License, "/") {
				return file.LocalPath, file.Path
			}
		}
	}

	var best *library.File

	for i, file := range files {
		if !licenseFile.MatchString(path.Base(file.LocalPath)) {
			continue
		}

		if best == nil || strings.Count(file.LocalPath, "/") < strings.Count(best.LocalPath, "/") {
			best = &files[i]
		}
	}

	if best == nil {
		return "", ""
	}

	return best.LocalPath, best.Path
}

// containsPath reports whether one of files has the path name.
func containsPath(files []LockedFile, name string) bool {
	return slices.ContainsFunc(files, func(file LockedFile) bool {
		return file.Path == name
	})
}

// providerName is the package name of a provider's type, such as "cdnjs".
func providerName(p library.Provider) string {
	name := fmt.Sprintf("%T", p)
	name = strings.TrimPrefix(name, "*")

	if pkg, _, ok := strings.Cut(name, "."); ok {
		return pkg
	}

	return name
}

// download returns the content at src, of at most maxVendorFile bytes.
func download(ctx context.Context, src string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("download %s: HTTP %s", src, resp.Status)
	}

	var buf bytes.Buffer

	n, err := io.Copy(&buf, io.LimitReader(resp.Body, maxVendorFile+1))
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", src, err)
	}

	if n > maxVendorFile {
		return nil, fmt.Errorf("download %s: larger than %d bytes", src, maxVendorFile)
	}

	return buf.Bytes(), nil
}

// writeFileAtomic writes data to a temporary file and renames it, so a
// failed write leaves the previous file.
func writeFileAtomic(filename string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return err
	}

	file, err := os.CreateTemp(filepath.Dir(filename), ".importmap-*")
	if err != nil {
		return err
	}

	defer func() { _ = os.Remove(file.Name()) }()
	defer func() { _ = file.Close() }()

	if _, err := file.Write(data); err != nil {
		return err
	}

	if err := file.Chmod(0o644); err != nil {
		return err
	}

	if err := file.Close(); err != nil {
		return err
	}

	return os.Rename(file.Name(), filename)
}
