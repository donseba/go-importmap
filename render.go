package importmap

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"html/template"
	"maps"
	"slices"
	"strings"

	"github.com/donseba/go-importmap/assets"
)

type (
	// RenderOption changes what Render produces; see RenderWith and
	// WithRenderOptions.
	RenderOption func(*renderConfig)

	renderConfig struct {
		integrity bool
		// stable leaves integrity out while the manifest reloads (see
		// WithStableIntegrity).
		stable  bool
		preload []string
		// nonce is the CSP nonce of the scripts and links Render writes
		// (WithNonce).
		nonce string
	}
)

// WithIntegrity adds the Subresource Integrity of local files, as WithAssets
// and FromFS know it: an "integrity" section in the import map and an
// integrity attribute on stylesheet and modulepreload links. Browsers then
// refuse a file whose content changed.
func WithIntegrity() RenderOption {
	return func(c *renderConfig) {
		c.integrity = true
	}
}

// WithStableIntegrity is WithIntegrity for pages that may be cached, by the
// browser or a server: while the manifest reloads (assets.Reload), nothing
// carries integrity, neither the import map nor its links nor Script and
// Stylesheet, since a reloading file changes under a cached page and a
// stale integrity makes the browser refuse the new content. A manifest
// that does not reload, such as one of an embedded file system, renders
// as with WithIntegrity. It holds for the whole render, also when
// WithIntegrity is given as well.
func WithStableIntegrity() RenderOption {
	return func(c *renderConfig) {
		c.integrity = true
		c.stable = true
	}
}

// WithNonce adds a nonce attribute with the Content-Security-Policy nonce
// of the response to the elements Render writes that the policy governs
// as scripts: the shim, the import map and the modulepreload links. Use it
// when the policy allows scripts by nonce ("script-src 'nonce-…'"), which
// must differ for every response, so a page rendered with it must not be
// cached; a cached page keeps an old nonce. For pages that are cached,
// allow the import map by its hash instead (ScriptHash). An empty nonce
// writes no attribute.
func WithNonce(nonce string) RenderOption {
	return func(c *renderConfig) {
		c.nonce = nonce
	}
}

// Styles returns a stylesheet link for each style, in name order.
func (im *ImportMap) Styles() (template.HTML, error) {
	if im.Structure.Styles == nil {
		return "", nil
	}

	config := im.renderConfig(nil)
	current, integrity := im.snapshot()

	var out strings.Builder

	for _, name := range slices.Sorted(maps.Keys(current.Styles)) {
		writeStylesheet(&out, config, integrity, current.Styles[name], ">")
	}

	//nolint:gosec // Fixed markup around escaped attribute values.
	return template.HTML(out.String()), nil
}

// Script returns a <script> tag that loads a local file at its URL (URL),
// for scripts a page loads directly rather than through the import map:
//
//	{{importmap_script "theme.js"}}
//	{{importmap_script "app.js" "type" "module" "data-api" $api}}
//
// With WithIntegrity among the render options (WithRenderOptions) it
// carries the file's Subresource Integrity and crossorigin="anonymous",
// as the import map does for its entries. attrs are more attributes as
// name and value pairs; see assets.ScriptTag.
func (im *ImportMap) Script(file string, attrs ...string) (template.HTML, error) {
	return assets.ScriptTag(im.URL(file), im.fileIntegrity(file), attrs...)
}

// Stylesheet returns a <link rel="stylesheet"> tag for a local file at its
// URL, with its Subresource Integrity under WithIntegrity, like Script:
//
//	{{importmap_style "admin.css"}}
func (im *ImportMap) Stylesheet(file string, attrs ...string) (template.HTML, error) {
	return assets.StylesheetTag(im.URL(file), im.fileIntegrity(file), attrs...)
}

// fileIntegrity is the integrity of a local file when integrity renders
// (WithIntegrity in WithRenderOptions) and the manifest knows the file.
func (im *ImportMap) fileIntegrity(file string) string {
	if im.manifest == nil || !im.renderConfig(nil).integrity {
		return ""
	}

	return im.manifest.Integrity(strings.TrimPrefix(file, "/"))
}

// Render returns an HTML snippet to use in a template
func (im *ImportMap) Render() (template.HTML, error) {
	return im.RenderWith()
}

// RenderWith returns what Render does, changed by opts: the stylesheet
// links in name order, the shim, the import map, and modulepreload links.
func (im *ImportMap) RenderWith(opts ...RenderOption) (template.HTML, error) {
	config := im.renderConfig(opts)
	current, integrity := im.snapshot()

	return im.renderHead(config, current, integrity)
}

// renderHead writes the head HTML of RenderWith for one reading of the
// map.
func (im *ImportMap) renderHead(config renderConfig, current structure, integrity map[string]string) (template.HTML, error) {
	var out strings.Builder

	for _, name := range slices.Sorted(maps.Keys(current.Styles)) {
		writeStylesheet(&out, config, integrity, current.Styles[name], "/>\n")
	}

	if im.shim != "" {
		out.WriteString(`<script async src="`)
		out.WriteString(template.HTMLEscapeString(im.shim))
		out.WriteString(`"`)
		writeAttribute(&out, "nonce", config.nonce)
		out.WriteString("></script>\n")
	}

	text, err := importMapText(config, current, integrity)
	if err != nil {
		return "", err
	}

	if text != "" {
		out.WriteString(`<script type="importmap"`)
		writeAttribute(&out, "nonce", config.nonce)
		out.WriteString(">")
		out.WriteString(text)
		out.WriteString("</script>")
	}

	// Module preloads follow the import map, which must come before any
	// module is fetched.
	for _, name := range config.preload {
		u, ok := current.Imports[name]
		if !ok {
			continue
		}

		out.WriteString("\n<link rel=\"modulepreload\" href=\"")
		out.WriteString(template.HTMLEscapeString(u))
		out.WriteString(`"`)

		if config.integrity {
			writeAttribute(&out, "integrity", integrity[u])
		}

		writeAttribute(&out, "nonce", config.nonce)
		out.WriteString("/>")
	}

	//nolint:gosec // Fixed markup around escaped attribute values and escaped JSON.
	return template.HTML(out.String()), nil
}

// ScriptHash returns the Content-Security-Policy hash of the import map
// script that RenderWith(opts...) writes, such as "'sha256-…'", quotes
// included, for a policy that allows that one inline script and no other:
//
//	script-src 'self' 'sha256-…'
//
// Without options it is the hash of what Render and {{importmap}} write.
// It hashes the text between <script type="importmap"> and </script>,
// made by the same code that renders it, so the two cannot differ; give
// it the options the page renders with, since WithIntegrity changes the
// text (WithNonce does not). The hash changes only when the map does, so
// it suits pages that are cached, unlike a nonce. While the manifest
// reloads (assets.Reload) the map, and so the hash, follows the files:
// take the hash and the page from the same moment, or use RenderWithHash.
// It is empty when there is no import map to render.
func (im *ImportMap) ScriptHash(opts ...RenderOption) string {
	config := im.renderConfig(opts)
	current, integrity := im.snapshot()

	// The text holds JSON of string maps, which cannot fail to encode.
	text, err := importMapText(config, current, integrity)
	if err != nil {
		return ""
	}

	return scriptHash(text)
}

// RenderWithHash returns what RenderWith(opts...) does together with the
// ScriptHash of its import map script, both from one reading of the map,
// so they match also while the manifest reloads. The hash is empty when
// there is no import map.
func (im *ImportMap) RenderWithHash(opts ...RenderOption) (template.HTML, string, error) {
	config := im.renderConfig(opts)
	current, integrity := im.snapshot()

	html, err := im.renderHead(config, current, integrity)
	if err != nil {
		return "", "", err
	}

	text, err := importMapText(config, current, integrity)
	if err != nil {
		return "", "", err
	}

	return html, scriptHash(text), nil
}

// importMapText is the text of the import map script: its JSON between
// newlines, or empty when there are no imports and no scopes. Render and
// ScriptHash both use it, so the hash is of the rendered bytes.
func importMapText(config renderConfig, current structure, integrity map[string]string) (string, error) {
	if len(current.Imports) == 0 && len(current.Scopes) == 0 {
		return "", nil
	}

	data := struct {
		Imports   map[string]string            `json:"imports"`
		Scopes    map[string]map[string]string `json:"scopes,omitempty"`
		Integrity map[string]string            `json:"integrity,omitempty"`
	}{
		Imports: current.Imports,
		Scopes:  current.Scopes,
	}

	if config.integrity {
		data.Integrity = importIntegrity(current, integrity)
	}

	// json escapes <, > and &, so no value can end the script.
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", err
	}

	return "\n" + string(b) + "\n", nil
}

// scriptHash is the CSP source expression of text: 'sha256-' and the
// standard base64 of its SHA-256, in single quotes; empty for no text.
func scriptHash(text string) string {
	if text == "" {
		return ""
	}

	sum := sha256.Sum256([]byte(text))

	return "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
}

// writeAttribute writes the attribute name with value, escaped, when value
// is not empty.
func writeAttribute(out *strings.Builder, name, value string) {
	if value == "" {
		return
	}

	out.WriteString(" ")
	out.WriteString(name)
	out.WriteString(`="`)
	out.WriteString(template.HTMLEscapeString(value))
	out.WriteString(`"`)
}

// renderConfig is the configuration of one render: the preloads of
// ModulePreload, changed by the options of WithRenderOptions and then by
// opts, without repeated preloads, and without integrity while the
// manifest reloads under WithStableIntegrity.
func (im *ImportMap) renderConfig(opts []RenderOption) renderConfig {
	config := renderConfig{preload: append([]string(nil), im.preload...)}

	for _, opt := range im.render {
		opt(&config)
	}

	for _, opt := range opts {
		opt(&config)
	}

	config.preload = unique(config.preload)

	if config.stable && im.manifest != nil && im.manifest.Reloads() {
		config.integrity = false
	}

	return config
}

// writeStylesheet writes a stylesheet link for href, with its integrity and
// crossorigin="anonymous" when config renders integrity and integrity
// knows href, closed with end.
func writeStylesheet(out *strings.Builder, config renderConfig, integrity map[string]string, href, end string) {
	out.WriteString(`<link rel="stylesheet" href="`)
	out.WriteString(template.HTMLEscapeString(href))
	out.WriteString(`"`)

	if value := integrity[href]; config.integrity && value != "" {
		writeAttribute(out, "integrity", value)
		out.WriteString(` crossorigin="anonymous"`)
	}

	out.WriteString(end)
}

// importIntegrity is the integrity of the imported URLs of current that
// have one in known.
func importIntegrity(current structure, known map[string]string) map[string]string {
	integrity := make(map[string]string)

	addIntegrity(integrity, known, current.Imports)

	for _, scope := range current.Scopes {
		addIntegrity(integrity, known, scope)
	}

	return integrity
}

// addIntegrity adds the integrity known gives for the URLs of entries to
// integrity.
func addIntegrity(integrity, known, entries map[string]string) {
	for _, u := range entries {
		if value := known[u]; value != "" {
			integrity[u] = value
		}
	}
}

// unique removes repeated names, keeping the first of each, in place.
func unique(names []string) []string {
	seen := make(map[string]bool, len(names))
	out := names[:0]

	for _, name := range names {
		if seen[name] {
			continue
		}

		seen[name] = true
		out = append(out, name)
	}

	return out
}
