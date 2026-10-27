package importmap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/donseba/go-importmap/assets"
	"github.com/donseba/go-importmap/client/cdnjs"
	"github.com/donseba/go-importmap/library"
)

var (
	defaultAssetsDir = "assets"
	defaultCacheDir  = ".importmap"
)

type (
	ImportMap struct {
		provider  library.Provider  // the js library provider
		packages  []library.Package // the library packages we want to include
		Structure structure         // the output structure

		rootDir   string
		assetsDir *string
		cacheDir  *string

		shim   string
		logger *slog.Logger

		// local maps import and style names to files below the assets
		// directory (or the file system of FromFS); their URLs come from
		// URL, so they follow WithURLFunc and WithAssets.
		local map[library.FileType]map[string]string
		// integrity holds the Subresource Integrity of local URLs.
		integrity map[string]string

		urlFunc  func(file string) string
		manifest *assets.Manifest

		// versioned lists the files whose plain URL the map points at their
		// versioned one (WithVersionedPaths); versionAll does so for every
		// file of the manifest.
		versioned  []string
		versionAll bool

		preload []string
		render  []RenderOption
	}

	structure struct {
		Imports map[string]string            `json:"imports,omitempty"`
		Scopes  map[string]map[string]string `json:"scopes,omitempty"`
		Styles  map[string]string            `json:"styles,omitempty"`
	}
)

// New returns a new instance of the ImportMap
func New() *ImportMap {
	return &ImportMap{
		Structure: structure{
			Imports: make(map[string]string),
			Scopes:  make(map[string]map[string]string),
			Styles:  make(map[string]string),
		},
	}
}

func NewDefaults() *ImportMap {
	return New().WithDefaults()
}

// WithDefaults sets the cdnjs provider, the "assets" assets directory and
// the ".importmap" cache directory. It sets no shim: current browsers read
// import maps themselves; see ShimPath.
func (im *ImportMap) WithDefaults() *ImportMap {
	im.CacheDir(defaultCacheDir)
	im.AssetsDir(defaultAssetsDir)
	im.WithProvider(cdnjs.New())
	return im
}

func (im *ImportMap) WithProvider(p library.Provider) *ImportMap {
	im.provider = p
	return im
}

// WithPackages replaces the configured packages with a copy of p.
func (im *ImportMap) WithPackages(p []library.Package) *ImportMap {
	im.packages = append([]library.Package(nil), p...)
	return im
}

func (im *ImportMap) WithPackage(p library.Package) *ImportMap {
	im.packages = append(im.packages, p)
	return im
}

func (im *ImportMap) WithLogger(logger *slog.Logger) *ImportMap {
	im.logger = logger
	return im
}

func (im *ImportMap) Clean() *ImportMap {
	cacheDir := defaultCacheDir
	if im.cacheDir != nil {
		cacheDir = *im.cacheDir
	}

	assetsDir := defaultAssetsDir
	if im.assetsDir != nil {
		assetsDir = *im.assetsDir
	}

	_ = os.RemoveAll(path.Join(im.rootDir, cacheDir))
	_ = os.RemoveAll(path.Join(im.rootDir, assetsDir))
	return im
}

func (im *ImportMap) CacheDir(dir string) *ImportMap {
	im.cacheDir = &dir
	return im
}

// AssetsDir sets the directory, below RootDir, where packages are published.
// Without WithURLFunc it is also the URL prefix of their files.
func (im *ImportMap) AssetsDir(dir string) *ImportMap {
	im.assetsDir = &dir
	return im
}

func (im *ImportMap) RootDir(dir string) *ImportMap {
	im.rootDir = dir
	return im
}

func (im *ImportMap) Shim() string {
	return im.shim
}

// ShimPath sets the URL of an ES module shim script, rendered before the
// import map, for browsers without import map support, such as the address
// of a vendored copy of es-module-shims. It is empty unless set.
func (im *ImportMap) ShimPath(sp string) *ImportMap {
	im.shim = sp
	return im
}

// WithImport maps name to url as it is, for a module the
// application serves itself, such as a route of its router, or a module on
// another host. integrity is the Subresource Integrity of the module at
// url, such as "sha384-…" from assets.Integrity, for a module whose
// integrity the manifest does not know: one served from another file
// system, such as a library's own copy, or from another host. With
// WithIntegrity among the render options it is in the map's integrity
// section, and on its modulepreload link, as for a local file. An empty
// integrity maps url without one.
func (im *ImportMap) WithImport(name, url, integrity string) *ImportMap {
	im.setURL(library.FileTypeJS, name, url)

	if integrity != "" {
		im.integrity[url] = integrity
	}

	return im
}

// WithFile maps name to a file below the assets directory (or the file
// system of FromFS), such as "app/main.js". Its URL comes from URL, so it
// follows WithURLFunc and is versioned with WithAssets. A .css file becomes
// a stylesheet, any other file an import.
func (im *ImportMap) WithFile(name, file string) *ImportMap {
	im.setLocal(fileKind(file), name, file)
	return im
}

// WithURLFunc sets how the URL of a local file is made, for example from a
// named route of a router r:
//
//	im.WithURLFunc(func(file string) string {
//		return r.RoutePathWithParams("assets", map[string]any{"file": file})
//	})
//
// file is relative to the assets directory, such as "htmx/htmx.esm.min.js".
// Without it the URL is "/" followed by the assets directory and the file.
// Entries already in the map are updated.
func (im *ImportMap) WithURLFunc(fn func(file string) string) *ImportMap {
	im.urlFunc = fn
	im.resolve()
	return im
}

// WithAssets versions local URLs with the manifest, which fingerprints the
// published files, and makes their integrity known for WithIntegrity. Use
// it with the files of the assets directory:
//
//	manifest, err := assets.Fingerprint(os.DirFS("assets"))
//	im.WithAssets(manifest)
//
// ServeFile then serves them. Entries already in the map are updated.
func (im *ImportMap) WithAssets(m *assets.Manifest) *ImportMap {
	im.manifest = m
	im.resolve()
	return im
}

// Manifest returns the fingerprints of the local files: those of the file
// system of FromFS, or the manifest of WithAssets; nil without either. Use
// it rather than fingerprinting the same files again, for example for
// Integrity or Files.
func (im *ImportMap) Manifest() *assets.Manifest {
	return im.manifest
}

// ServeFile answers a GET or HEAD request for a local file, for routers
// that take the file name from a path parameter:
//
//	mux.HandleFunc("GET /assets/{file...}", func(w http.ResponseWriter, r *http.Request) {
//		im.ServeFile(w, r, r.PathValue("file"))
//	})
//
// It serves the file system of FromFS or the manifest of WithAssets (see
// assets.Manifest.ServeFile); without either every file is not found.
func (im *ImportMap) ServeFile(w http.ResponseWriter, r *http.Request, file string) {
	if im.manifest == nil {
		http.NotFound(w, r)

		return
	}

	im.manifest.ServeFile(w, r, file)
}

// WithVersionedPaths maps the plain URL of each local file, without its
// version, to its versioned URL. A script that loads a file by a relative
// path, such as import "./chunk.js" or import.meta.resolve("./icons.svg"),
// so gets the versioned URL, which the browser may cache for a year, while
// the import map is the only place that knows the version. Without
// patterns it maps every file of the manifest (FromFS or WithAssets),
// also files added later to a manifest made with assets.Reload, except
// the lock file (LockFile) and licence texts (LICENSE, LICENCE, COPYING,
// NOTICE, with or without an extension), which no script loads:
//
//	im.WithVersionedPaths()                         // every local file
//	im.WithVersionedPaths("editor/prosemirror.js") // only these
//	im.WithVersionedPaths("**/*.js", "**/*.svg")   // files that match
//	im.WithVersionedPaths("!vendor/**")             // all but these
//
// A pattern matches a file's whole path, slash-separated, with the syntax
// of path.Match; "**" stands for any number of directories. A pattern
// that starts with "!" leaves out the files it matches; with only such
// patterns the others are every file but the lock and licences.
//
// The URLs follow WithURLFunc. Files the manifest does not know are left
// out, and a name mapped with WithImport or WithFile wins over
// such an entry. Calls add to each other.
func (im *ImportMap) WithVersionedPaths(patterns ...string) *ImportMap {
	if len(patterns) == 0 {
		im.versionAll = true
	}

	im.versioned = append(im.versioned, patterns...)

	return im
}

// ModulePreload adds a <link rel="modulepreload"> for each named import to
// what Render produces, so the browser fetches those modules early.
func (im *ImportMap) ModulePreload(names ...string) *ImportMap {
	im.preload = append(im.preload, names...)
	return im
}

// WithRenderOptions sets the options Render, Styles and FuncMap use, before
// the options of a RenderWith call.
func (im *ImportMap) WithRenderOptions(opts ...RenderOption) *ImportMap {
	im.render = append(im.render, opts...)
	return im
}

// URL returns the address of a file below the assets directory (or the file
// system of FromFS): from WithURLFunc, or "/" followed by the assets
// directory and the file, with the version from WithAssets.
func (im *ImportMap) URL(file string) string {
	file = strings.TrimPrefix(file, "/")

	u := im.plainURL(file)

	if im.manifest != nil {
		u = assets.Versioned(u, im.manifest.Version(file))
	}

	return u
}

// plainURL is the address of a local file without its version.
func (im *ImportMap) plainURL(file string) string {
	if im.urlFunc != nil {
		return im.urlFunc(file)
	}

	return path.Join("/", im.assetsDirOrDefault(), file)
}

// assetsDirOrDefault is the directory of AssetsDir, or "assets" when none
// is set.
func (im *ImportMap) assetsDirOrDefault() string {
	if im.assetsDir != nil {
		return *im.assetsDir
	}

	return defaultAssetsDir
}

// FuncMap gives templates the import map:
//
//	{{importmap}}                         what Render produces, for the head
//	{{importmap_nonce $nonce}}            the same with a CSP nonce (WithNonce)
//	{{importmap_url "app/main.js"}}       the URL of a local file, as URL gives it
//	{{importmap_script "theme.js"}}       a <script> tag for a local file (Script)
//	{{importmap_style "admin.css"}}       a stylesheet link for a local file (Stylesheet)
//
// importmap_script and importmap_style add the file's integrity under
// WithIntegrity, and take more attributes as name and value pairs, such
// as "nonce" $nonce.
func (im *ImportMap) FuncMap() template.FuncMap {
	return template.FuncMap{
		"importmap":        im.Render,
		"importmap_nonce":  im.renderNonce,
		"importmap_url":    im.URL,
		"importmap_script": im.Script,
		"importmap_style":  im.Stylesheet,
	}
}

// renderNonce is what Render produces with the CSP nonce of the response,
// for {{importmap_nonce}}.
func (im *ImportMap) renderNonce(nonce string) (template.HTML, error) {
	return im.RenderWith(WithNonce(nonce))
}

// setLocal maps name to a local file.
func (im *ImportMap) setLocal(kind library.FileType, name, file string) {
	im.init()

	file = strings.TrimPrefix(file, "/")
	im.local[kind][name] = file
	im.mapFile(im.Structure, im.integrity, kind, name, file)
}

// mapFile points name at the URL of a local file in s, and records the
// file's integrity in integrity when the manifest knows it.
func (im *ImportMap) mapFile(s structure, integrity map[string]string, kind library.FileType, name, file string) {
	u := im.URL(file)
	s.entries(kind)[name] = u

	if im.manifest == nil {
		return
	}

	if value := im.manifest.Integrity(file); value != "" {
		integrity[u] = value
	}
}

// setURL maps name to a URL as it is.
func (im *ImportMap) setURL(kind library.FileType, name, u string) {
	im.init()

	delete(im.local[kind], name)
	im.Structure.entries(kind)[name] = u
}

// resolve updates the URLs of local files.
func (im *ImportMap) resolve() {
	im.init()

	for kind, files := range im.local {
		for name, file := range files {
			im.setLocal(kind, name, file)
		}
	}
}

// entries are the styles of s for a stylesheet and its imports for any
// other kind of file.
func (s structure) entries(kind library.FileType) map[string]string {
	if kind == library.FileTypeCSS {
		return s.Styles
	}

	return s.Imports
}

// fileKind is how the map holds a local file: a .css file as a style, any
// other file as an import.
func fileKind(file string) library.FileType {
	if library.ExtractFileType(file) == library.FileTypeCSS {
		return library.FileTypeCSS
	}

	return library.FileTypeJS
}

// mappable reports whether the map holds files of kind: scripts as imports
// and stylesheets as styles.
func mappable(kind library.FileType) bool {
	return kind == library.FileTypeJS || kind == library.FileTypeCSS
}

// init makes the maps the import map writes to when they are missing, as
// in an ImportMap that was not made with New.
func (im *ImportMap) init() {
	if im.Structure.Imports == nil {
		im.Structure.Imports = make(map[string]string)
	}

	if im.Structure.Styles == nil {
		im.Structure.Styles = make(map[string]string)
	}

	if im.local == nil {
		im.local = map[library.FileType]map[string]string{
			library.FileTypeJS:  {},
			library.FileTypeCSS: {},
		}
	}

	if im.integrity == nil {
		im.integrity = make(map[string]string)
	}
}

// CacheOrFetch maps the packages from the cache and the assets directory,
// rebuilding missing assets from the cache, and fetches only the packages
// without a cache. A package without a version has no cache to find, so pin
// versions.
func (im *ImportMap) CacheOrFetch(ctx context.Context) error {
	im.logInfo(ctx, "checking cache and assets for packages")

	if im.cacheDir == nil || im.assetsDir == nil {
		return errors.New("cacheDir and assetsDir must be set")
	}

	if err := im.checkPatterns(); err != nil {
		return err
	}

	for i := range im.packages {
		if err := im.cacheOrFetchPackage(ctx, &im.packages[i]); err != nil {
			return err
		}
	}

	return nil
}

// cacheOrFetchPackage maps one package from the cache and the assets
// directory, rebuilding its missing assets from the cache, or fetches it
// when it has no cache.
func (im *ImportMap) cacheOrFetchPackage(ctx context.Context, pkg *library.Package) error {
	im.logInfo(ctx, "checking package cache and assets", "package", pkg.Name)

	if rawOnly(pkg) {
		im.applyRaw(pkg)
		return nil
	}

	// Local files are built by the application, not fetched.
	if pkg.Local {
		return nil
	}

	if !pkg.HasCache(im.rootDir, *im.cacheDir) {
		im.logInfo(ctx, "cache not found, fetching", "package", pkg.Name)

		// fetchPackage builds the cache and assets of this package only.
		return im.fetchPackage(ctx, pkg)
	}

	if err := im.restoreAssets(ctx, pkg); err != nil {
		return err
	}

	if err := im.mapAssets(ctx, pkg); err != nil {
		return err
	}

	im.applyRaw(pkg)

	return nil
}

// restoreAssets builds the assets of pkg that are missing from its cache.
func (im *ImportMap) restoreAssets(ctx context.Context, pkg *library.Package) error {
	cacheFiles, err := pkg.CacheFiles(im.rootDir, *im.cacheDir)
	if err != nil {
		im.logError(ctx, "error reading cache for assets", "package", pkg.Name, "error", err)

		return err
	}

	for _, file := range cacheFiles {
		if pkg.HasAssetFile(im.rootDir, *im.assetsDir, file.LocalPath) {
			continue
		}

		if _, ok := importName(pkg, file.LocalPath); !ok {
			continue
		}

		im.logInfo(ctx, "asset not found, building from cache", "package", pkg.Name, "file", file.LocalPath)

		err = pkg.MakeAssetsContext(ctx, im.rootDir, *im.cacheDir, *im.assetsDir, file.LocalPath, file.Path)
		if err != nil {
			return err
		}
	}

	return nil
}

// mapAssets maps the scripts and stylesheets of pkg that are published in
// the assets directory.
func (im *ImportMap) mapAssets(ctx context.Context, pkg *library.Package) error {
	assetFiles, _, err := pkg.AssetsAt(im.rootDir, *im.assetsDir, "")
	if err != nil {
		im.logError(ctx, "error reading assets", "package", pkg.Name, "error", err)

		return err
	}

	for _, file := range assetFiles {
		as, ok := importName(pkg, file.LocalPath)
		if ok && mappable(file.Type) {
			im.setLocal(file.Type, as, path.Join(pkg.Name, file.LocalPath))
		}
	}

	return nil
}

// checkPatterns returns an error for the first package with a Require
// pattern that is not valid, before anything is fetched or written.
func (im *ImportMap) checkPatterns() error {
	for i := range im.packages {
		if err := im.packages[i].Require.Check(); err != nil {
			return fmt.Errorf("package %s: %w", im.packages[i].Name, err)
		}
	}

	return nil
}

// Fetch asks the providers for every package, fills the cache and the assets
// directory with the files not there yet, and maps them.
func (im *ImportMap) Fetch(ctx context.Context) error {
	if err := im.checkPatterns(); err != nil {
		return err
	}

	for i := range im.packages {
		if err := im.fetchPackage(ctx, &im.packages[i]); err != nil {
			return err
		}
	}

	return nil
}

// fetchPackage fetches one package; a package without a version gets the
// version the provider chose.
func (im *ImportMap) fetchPackage(ctx context.Context, pkg *library.Package) error {
	im.logInfo(ctx, "fetching assets", "package", pkg.Name)

	// Raw imports are URLs, and local files are the application's own;
	// there is nothing to fetch.
	if rawOnly(pkg) {
		im.applyRaw(pkg)
		return nil
	}

	if pkg.Local {
		return nil
	}

	provider := im.providerFor(pkg)
	if provider == nil {
		return fmt.Errorf("package %q has no provider", pkg.Name)
	}

	files, err := fetchFiles(ctx, provider, pkg)
	if err != nil {
		return err
	}

	if err := im.buildCache(ctx, pkg, files); err != nil {
		return err
	}

	if err := im.publishFiles(ctx, pkg, files); err != nil {
		return err
	}

	im.applyRaw(pkg)

	return nil
}

// fetchFiles asks provider for the files of pkg. A package without a
// version gets the version the provider chose.
func fetchFiles(ctx context.Context, provider library.Provider, pkg *library.Package) (library.Files, error) {
	files, version, err := provider.FetchPackageFiles(ctx, pkg.Name, pkg.Version)
	if err != nil {
		return nil, err
	}

	if pkg.Version == "" {
		pkg.Version = version
	}

	return files, nil
}

// buildCache downloads the files of pkg the cache lacks into it; without a
// cache directory it does nothing.
func (im *ImportMap) buildCache(ctx context.Context, pkg *library.Package, files library.Files) error {
	if im.cacheDir == nil {
		return nil
	}

	im.logInfo(ctx, "building cache", "package", pkg.Name, "version", pkg.Version)

	for _, file := range files {
		if pkg.HasCacheFile(im.rootDir, *im.cacheDir, file.LocalPath) {
			continue
		}

		err := pkg.MakeCacheContext(ctx, im.rootDir, *im.cacheDir, file.LocalPath, file.Path)
		if err != nil {
			return err
		}
	}

	return nil
}

// publishFiles puts the files of pkg its Require patterns select in the
// assets directory, from the cache or else from the provider, when they
// are not there yet, and maps the scripts and stylesheets among them.
// Without an assets directory the map points at the provider's URLs.
func (im *ImportMap) publishFiles(ctx context.Context, pkg *library.Package, files library.Files) error {
	var cacheDir string
	if im.cacheDir != nil {
		cacheDir = *im.cacheDir
	}

	im.logInfo(ctx, "building assets", "package", pkg.Name, "version", pkg.Version)

	for _, file := range files {
		as, ok := importName(pkg, file.LocalPath)
		if !ok {
			continue
		}

		if im.assetsDir == nil {
			if kind := library.ExtractFileType(file.Path); mappable(kind) {
				im.setURL(kind, as, file.Path)
			}

			continue
		}

		if !pkg.HasAssetFile(im.rootDir, *im.assetsDir, file.LocalPath) {
			err := pkg.MakeAssetsContext(ctx, im.rootDir, cacheDir, *im.assetsDir, file.LocalPath, file.Path)
			if err != nil {
				return err
			}
		}

		if kind := library.ExtractFileType(file.LocalPath); mappable(kind) {
			im.setLocal(kind, as, path.Join(pkg.Name, file.LocalPath))
		}
	}

	return nil
}

// providerFor is the provider of pkg, or the map's provider when the
// package has none of its own.
func (im *ImportMap) providerFor(pkg *library.Package) library.Provider {
	if pkg.Provider != nil {
		return pkg.Provider
	}

	return im.provider
}

// logInfo logs an informational message when a logger is set.
func (im *ImportMap) logInfo(ctx context.Context, msg string, args ...any) {
	if im.logger != nil {
		im.logger.InfoContext(ctx, msg, args...)
	}
}

// logError logs an error when a logger is set.
func (im *ImportMap) logError(ctx context.Context, msg string, args ...any) {
	if im.logger != nil {
		im.logger.ErrorContext(ctx, msg, args...)
	}
}

// applyRaw maps the raw imports of pkg.
func (im *ImportMap) applyRaw(pkg *library.Package) {
	for _, req := range pkg.Require {
		if req.Raw != "" {
			im.setURL(library.FileTypeJS, req.Name(), req.Raw)
		}
	}
}

// importName is the name a package file is mapped as: the name of the
// Include it matches, or its path when the package requires no files.
func importName(pkg *library.Package, localPath string) (string, bool) {
	if len(pkg.Require) == 0 {
		return localPath, true
	}

	req := pkg.Require.Get(localPath)
	if req == nil {
		return "", false
	}

	return req.Name(), true
}

// rawOnly reports whether every file pkg requires is a raw URL, so the
// provider is not needed.
func rawOnly(pkg *library.Package) bool {
	if len(pkg.Require) == 0 {
		return false
	}

	for _, req := range pkg.Require {
		if req.Raw == "" || req.File != "" {
			return false
		}
	}

	return true
}

// Marshal returns the import map as JSON: the Structure with the entries
// of WithVersionedPaths.
func (im *ImportMap) Marshal() ([]byte, error) {
	current, _ := im.snapshot()

	return json.Marshal(current)
}

// MarshalIndent returns the import map as JSON in a pretty form.
func (im *ImportMap) MarshalIndent() ([]byte, error) {
	current, _ := im.snapshot()

	return json.MarshalIndent(current, "", "  ")
}

// Imports return the structure in JSON/HTML.
func (im *ImportMap) Imports() (template.HTML, error) {
	current, _ := im.snapshot()

	var in = struct {
		Imports map[string]string `json:"imports"`
	}{
		Imports: current.Imports,
	}

	b, err := json.Marshal(in)
	if err != nil {
		return "", err
	}

	return template.HTML(b), nil
}

// ImportsIndent return the structure in JSON/HTML.
func (im *ImportMap) ImportsIndent() (template.HTML, error) {
	current, _ := im.snapshot()

	var in = struct {
		Imports map[string]string `json:"imports"`
	}{
		Imports: current.Imports,
	}

	b, err := json.MarshalIndent(in, "", "  ")
	if err != nil {
		return "", err
	}

	return template.HTML(b), nil
}

func (im *ImportMap) Scopes() (template.HTML, error) {
	b, err := json.MarshalIndent(im.Structure.Scopes, "", "  ")
	if err != nil {
		return "", err
	}

	return template.HTML(b), nil
}

// snapshot is the import map as it renders, with the integrity of its
// URLs: the Structure, with the entries of WithVersionedPaths, and with
// the URLs of local files as they are now when the manifest reloads
// (assets.Reload). Without either it is the Structure itself.
func (im *ImportMap) snapshot() (structure, map[string]string) {
	reloads := im.manifest != nil && im.manifest.Reloads()
	versioned := im.manifest != nil && (im.versionAll || len(im.versioned) > 0)

	if !reloads && !versioned {
		return im.Structure, im.integrity
	}

	current := structure{
		Imports: maps.Clone(im.Structure.Imports),
		Scopes:  im.Structure.Scopes,
		Styles:  maps.Clone(im.Structure.Styles),
	}

	if current.Imports == nil {
		current.Imports = make(map[string]string)
	}

	integrity := maps.Clone(im.integrity)
	if integrity == nil {
		integrity = make(map[string]string)
	}

	if reloads {
		for kind, files := range im.local {
			for name, file := range files {
				im.mapFile(current, integrity, kind, name, file)
			}
		}
	}

	if versioned {
		im.mapVersionedPaths(current, integrity)
	}

	return current, integrity
}

// mapVersionedPaths points the plain URL of each file of
// WithVersionedPaths at its versioned URL in current, with the integrity
// of a script, unless a name of the map already is that plain URL.
func (im *ImportMap) mapVersionedPaths(current structure, integrity map[string]string) {
	for _, file := range im.versionedFiles() {
		version := im.manifest.Version(file)
		if version == "" {
			continue
		}

		plain := im.plainURL(file)
		if _, taken := current.Imports[plain]; taken {
			continue
		}

		u := assets.Versioned(plain, version)
		current.Imports[plain] = u

		if library.ExtractFileType(file) == library.FileTypeJS {
			integrity[u] = im.manifest.Integrity(file)
		}
	}
}
