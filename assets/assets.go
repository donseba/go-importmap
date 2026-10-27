// Package assets fingerprints a file system of static files, such as an
// embed.FS of vendored packages, and serves it with caching that suits
// versioned URLs.
//
// Fingerprint hashes every file once. URL then gives a file's address with
// its version (?v=...), Integrity its Subresource Integrity value, and
// ServeFile serves a file with a strong ETag: a request for the current
// version may be cached for a year, any other request revalidates.
//
//	manifest, err := assets.Fingerprint(static)
//	manifest.WithURLFunc(func(file string) string { return "/static/" + file })
//	mux.HandleFunc("GET /static/{file...}", func(w http.ResponseWriter, r *http.Request) {
//		manifest.ServeFile(w, r, r.PathValue("file"))
//	})
//
// The files must not change once fingerprinted; fingerprint again to pick up
// new content, or, while developing, fingerprint with Reload, which notices
// changed, added and removed files by their names, sizes and modification
// times:
//
//	manifest, err := assets.Fingerprint(os.DirFS("static"), assets.Reload())
package assets

import (
	"bytes"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"io"
	"io/fs"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

// immutable is the Cache-Control of a request for a file's current version.
const immutable = "public, max-age=31536000, immutable"

// versionLength is the number of hash bytes in a version and an ETag.
const versionLength = 8

type (
	// Manifest is the fingerprint of every file in a file system. It is
	// safe for concurrent use once configured (WithURLFunc).
	Manifest struct {
		fsys    fs.FS
		urlFunc func(file string) string
		reload  bool
		// only lists the files to fingerprint (Only); nil fingerprints
		// every file.
		only []string

		mu    sync.RWMutex
		files map[string]fingerprint
		// stamp sums the names, sizes and modification times of the files
		// when they were fingerprinted, with Reload.
		stamp uint64
	}

	fingerprint struct {
		version   string
		integrity string
		// size and modified are the file's size and modification time
		// when it was hashed, so ServeFile notices a file that changed
		// since.
		size     int64
		modified time.Time
	}

	// Option changes how Fingerprint fingerprints; see Reload.
	Option func(*Manifest)
)

// Reload makes the manifest look at the file system again on every use,
// cheaply, by the names, sizes and modification times of its files, and
// fingerprint it again when any of them changed or a file was added or
// removed. Edited files so get new versions without a restart. It is meant
// for development with a file system on disk, such as os.DirFS; an embed.FS
// never changes. A change that keeps a file's size and modification time
// goes unnoticed.
func Reload() Option {
	return func(m *Manifest) {
		m.reload = true
	}
}

// Only fingerprints the named files of the file system alone, such as one
// stylesheet in a folder that also holds templates, rather than every
// file. Names are slash-separated paths in the file system, such as
// "css/site.css"; a leading slash is ignored. Fingerprint fails when one
// of them is not a regular file. The manifest knows, versions and serves
// those files only. Calls add to each other.
//
//	manifest, err := assets.Fingerprint(module.Files, assets.Only("static/module.css"))
func Only(files ...string) Option {
	return func(m *Manifest) {
		if m.only == nil {
			m.only = []string{}
		}

		for _, file := range files {
			m.only = append(m.only, clean(file))
		}
	}
}

// Fingerprint hashes every file in fsys with SHA-384, or the files of Only.
// Hidden files and directories, whose name starts with a dot, are left out
// unless Only names them.
func Fingerprint(fsys fs.FS, opts ...Option) (*Manifest, error) {
	m := &Manifest{fsys: fsys}

	for _, opt := range opts {
		opt(m)
	}

	for _, file := range m.only {
		if !fs.ValidPath(file) || file == "." {
			return nil, fmt.Errorf("fingerprint assets: invalid file name %q", file)
		}
	}

	if m.reload {
		m.stamp = m.stampFiles()
	}

	files, err := m.fingerprintFiles()
	if err != nil {
		return nil, err
	}

	m.files = files

	return m, nil
}

// Reloads reports whether the manifest was made with Reload.
func (m *Manifest) Reloads() bool {
	return m.reload
}

// fingerprints returns the fingerprints of the files: with Reload, of the
// files as they are now.
func (m *Manifest) fingerprints() map[string]fingerprint {
	if !m.reload {
		return m.files
	}

	current := m.stampFiles()

	m.mu.RLock()
	files := m.files
	known := m.stamp
	m.mu.RUnlock()

	if current == known {
		return files
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Another request may have fingerprinted the files meanwhile.
	if current == m.stamp {
		return m.files
	}

	refreshed, err := m.fingerprintFiles()
	if err != nil {
		// A file may be half written; keep the fingerprints and try again
		// on the next use.
		return m.files
	}

	m.files = refreshed
	m.stamp = current

	return m.files
}

// stampFiles sums the names, sizes and modification times of the files the
// manifest fingerprints, without reading them.
func (m *Manifest) stampFiles() uint64 {
	hash := fnv.New64a()

	// A file that cannot be read is left out: fingerprinting reports it.
	_ = m.eachFile(func(name string, info fs.FileInfo, err error) error {
		if err == nil {
			_, _ = fmt.Fprintf(hash, "%s\x00%d\x00%d\x00", name, info.Size(), info.ModTime().UnixNano())
		}

		return nil
	})

	return hash.Sum64()
}

// fingerprintFiles hashes the files the manifest fingerprints: those of
// Only, or every file that is not hidden.
func (m *Manifest) fingerprintFiles() (map[string]fingerprint, error) {
	files := make(map[string]fingerprint)

	err := m.eachFile(func(name string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular file", name)
		}

		fp, err := fingerprintFile(m.fsys, name, info)
		if err != nil {
			return err
		}

		files[name] = fp

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("fingerprint assets: %w", err)
	}

	return files, nil
}

// eachFile calls fn for each file the manifest fingerprints, with its size
// and modification time in info or the error of reading them: the files of
// Only, or every regular file that is not hidden. Hidden files and
// directories, whose name starts with a dot, are left out, directories with
// all they hold. The size and time are read before the content, so a file
// that changes while it is hashed looks changed to ServeFile. An error fn
// returns ends the walk.
func (m *Manifest) eachFile(fn func(name string, info fs.FileInfo, err error) error) error {
	if m.only != nil {
		for _, name := range m.only {
			info, err := fs.Stat(m.fsys, name)
			if stop := fn(name, info, err); stop != nil {
				return stop
			}
		}

		return nil
	}

	return fs.WalkDir(m.fsys, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return fn(name, nil, err)
		}

		if name != "." && strings.HasPrefix(entry.Name(), ".") {
			if entry.IsDir() {
				return fs.SkipDir
			}

			return nil
		}

		if !entry.Type().IsRegular() {
			return nil
		}

		info, err := entry.Info()

		return fn(name, info, err)
	})
}

// fingerprintFile hashes the file name in fsys, whose size and modification
// time info gives.
func fingerprintFile(fsys fs.FS, name string, info fs.FileInfo) (fingerprint, error) {
	sum, err := hashFile(fsys, name)
	if err != nil {
		return fingerprint{}, err
	}

	fp := fingerprintOf(sum)
	fp.size = info.Size()
	fp.modified = info.ModTime()

	return fp, nil
}

// fingerprintOf is the fingerprint of a SHA-384 sum.
func fingerprintOf(sum []byte) fingerprint {
	return fingerprint{
		version:   hex.EncodeToString(sum[:versionLength]),
		integrity: "sha384-" + base64.StdEncoding.EncodeToString(sum),
	}
}

// MustFingerprint is Fingerprint for a file system that is known to be
// readable, such as an embed.FS; it panics on an error.
func MustFingerprint(fsys fs.FS, opts ...Option) *Manifest {
	m, err := Fingerprint(fsys, opts...)
	if err != nil {
		panic(err)
	}

	return m
}

// Integrity returns the Subresource Integrity value of data
// ("sha384-..."), as Manifest.Integrity gives for a file.
func Integrity(data []byte) string {
	sum := sha512.Sum384(data)

	return "sha384-" + base64.StdEncoding.EncodeToString(sum[:])
}

// Versioned adds version to rawURL as the query parameter v, before a
// fragment. An empty version leaves rawURL as it is.
func Versioned(rawURL, version string) string {
	if version == "" {
		return rawURL
	}

	rest, fragment, hasFragment := strings.Cut(rawURL, "#")

	separator := "?"
	if strings.Contains(rest, "?") {
		separator = "&"
	}

	rest += separator + "v=" + version

	if hasFragment {
		rest += "#" + fragment
	}

	return rest
}

// WithURLFunc sets the address of a file without its version, for example
// from a router. The default is "/" followed by the file name. Set it before
// the manifest is used.
func (m *Manifest) WithURLFunc(fn func(file string) string) *Manifest {
	m.urlFunc = fn

	return m
}

// Files returns the names of the fingerprinted files in order.
func (m *Manifest) Files() []string {
	files := m.fingerprints()
	names := slices.AppendSeq(make([]string, 0, len(files)), maps.Keys(files))

	slices.Sort(names)

	return names
}

// Has reports whether file was fingerprinted.
func (m *Manifest) Has(file string) bool {
	_, ok := m.fingerprints()[clean(file)]

	return ok
}

// Version returns a short hash of file's content, or "" for an unknown file.
func (m *Manifest) Version(file string) string {
	return m.fingerprints()[clean(file)].version
}

// Integrity returns the Subresource Integrity value of file
// ("sha384-..."), or "" for an unknown file.
func (m *Manifest) Integrity(file string) string {
	return m.fingerprints()[clean(file)].integrity
}

// TagIntegrity returns the Subresource Integrity a page should carry for
// file, as Stylesheet writes it: Integrity, or "" while the manifest
// reloads (Reload). A reloading file changes under pages that may be
// cached, by the browser or a server, and a stale integrity makes the
// browser refuse the new content. An unknown file gives "".
func (m *Manifest) TagIntegrity(file string) string {
	if m.reload {
		return ""
	}

	return m.Integrity(file)
}

// URL returns the address of file with its version, such as
// "/htmx/htmx.esm.min.js?v=3f2a...". An unknown file gets its address
// without a version.
func (m *Manifest) URL(file string) string {
	file = clean(file)

	return Versioned(m.PlainURL(file), m.Version(file))
}

// PlainURL returns the address of file without its version: from
// WithURLFunc, or "/" followed by the file name.
func (m *Manifest) PlainURL(file string) string {
	file = clean(file)

	if m.urlFunc != nil {
		return m.urlFunc(file)
	}

	return "/" + file
}

// ServeFile answers a GET or HEAD request for file, for routers that take
// the file name from a path parameter. Responses carry a strong ETag, so
// browsers revalidate cheaply. A request whose v parameter is the file's
// current version may be cached for a year; any other one must revalidate.
// Unknown files are not found.
//
// A file whose size or modification time differs from when it was
// fingerprinted, such as one edited on disk since, is hashed again before
// it is served, so its ETag and version are always those of the content
// sent: a request for the old version then must revalidate instead of
// caching the new content for a year under the old version.
func (m *Manifest) ServeFile(w http.ResponseWriter, r *http.Request, file string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)

		return
	}

	file = clean(file)

	fp, ok := m.fingerprints()[file]
	if !ok {
		http.NotFound(w, r)

		return
	}

	content, info, err := m.open(file)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		return
	}

	if closer, ok := content.(io.Closer); ok {
		defer func() { _ = closer.Close() }()
	}

	var modified time.Time
	if info != nil {
		modified = info.ModTime()
	}

	if info == nil || info.Size() != fp.size || !info.ModTime().Equal(fp.modified) {
		// The file changed since it was fingerprinted: label it with the
		// version of what is sent.
		fp, err = fingerprintContent(content)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

			return
		}
	}

	header := w.Header()
	header.Set("ETag", `"`+fp.version+`"`)
	header.Set("X-Content-Type-Options", "nosniff")

	if r.URL.Query().Get("v") == fp.version {
		header.Set("Cache-Control", immutable)
	} else {
		header.Set("Cache-Control", "no-cache")
	}

	http.ServeContent(w, r, file, modified, content)
}

// open returns file as a ReadSeeker, with its size and modification time
// (nil when they are unknown).
func (m *Manifest) open(file string) (io.ReadSeeker, fs.FileInfo, error) {
	f, err := m.fsys.Open(file)
	if err != nil {
		return nil, nil, err
	}

	info, err := f.Stat()
	if err != nil {
		info = nil
	}

	if seeker, ok := f.(io.ReadSeeker); ok {
		return seeker, info, nil
	}

	defer func() { _ = f.Close() }()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, nil, err
	}

	return bytes.NewReader(data), info, nil
}

// clean turns a request path or file name into a name in the file system.
func clean(file string) string {
	return strings.TrimPrefix(file, "/")
}

// fingerprintContent is the fingerprint of what content reads, after which
// it seeks content back to the start.
func fingerprintContent(content io.ReadSeeker) (fingerprint, error) {
	sum, err := sum384(content)
	if err != nil {
		return fingerprint{}, err
	}

	if _, err := content.Seek(0, io.SeekStart); err != nil {
		return fingerprint{}, err
	}

	return fingerprintOf(sum), nil
}

// hashFile is the SHA-384 sum of the file name in fsys.
func hashFile(fsys fs.FS, name string) ([]byte, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}

	defer func() { _ = f.Close() }()

	return sum384(f)
}

// sum384 is the SHA-384 sum of what r reads.
func sum384(r io.Reader) ([]byte, error) {
	hash := sha512.New384()

	if _, err := io.Copy(hash, r); err != nil {
		return nil, err
	}

	return hash.Sum(nil), nil
}
