package assets

import (
	"fmt"
	"html/template"
	"regexp"
	"slices"
	"strings"
)

// attributeName matches the attribute names ScriptTag and StylesheetTag
// accept.
var attributeName = regexp.MustCompile(`^[A-Za-z_:][-A-Za-z0-9_:.]*$`)

// ScriptTag returns a <script> tag that loads src, with the Subresource
// Integrity value integrity ("sha384-...") and crossorigin="anonymous" when
// integrity is not empty, so the browser refuses a file whose content
// changed. attrs are more attributes as name and value pairs, such as
// "type", "module" or "defer", ""; an empty value renders the name alone.
// A crossorigin attribute among attrs replaces the default one; src and
// integrity cannot be set through attrs.
//
//	tag, err := assets.ScriptTag(manifest.URL("app.js"), manifest.Integrity("app.js"), "type", "module")
func ScriptTag(src, integrity string, attrs ...string) (template.HTML, error) {
	var out strings.Builder

	out.WriteString(`<script src="`)
	out.WriteString(template.HTMLEscapeString(src))
	out.WriteString(`"`)

	if err := writeAttributes(&out, integrity, attrs, "src"); err != nil {
		return "", err
	}

	out.WriteString(`></script>`)

	//nolint:gosec // Fixed markup around escaped attribute values.
	return template.HTML(out.String()), nil
}

// StylesheetTag returns a <link rel="stylesheet"> tag for href, with the
// Subresource Integrity value integrity and crossorigin="anonymous" when
// integrity is not empty. attrs are more attributes as name and value
// pairs, such as "media", "print"; rel, href and integrity cannot be set
// through attrs. See ScriptTag.
func StylesheetTag(href, integrity string, attrs ...string) (template.HTML, error) {
	var out strings.Builder

	out.WriteString(`<link rel="stylesheet" href="`)
	out.WriteString(template.HTMLEscapeString(href))
	out.WriteString(`"`)

	if err := writeAttributes(&out, integrity, attrs, "rel", "href"); err != nil {
		return "", err
	}

	out.WriteString(`>`)

	//nolint:gosec // Fixed markup around escaped attribute values.
	return template.HTML(out.String()), nil
}

// writeAttributes writes the integrity attributes and attrs. reserved are
// the names attrs may not set besides integrity.
func writeAttributes(out *strings.Builder, integrity string, attrs []string, reserved ...string) error {
	if len(attrs)%2 != 0 {
		return fmt.Errorf("assets: attributes come in name and value pairs, got %d values", len(attrs))
	}

	crossOrigin := false

	for i := 0; i < len(attrs); i += 2 {
		name := strings.ToLower(attrs[i])

		if !attributeName.MatchString(name) {
			return fmt.Errorf("assets: invalid attribute name %q", attrs[i])
		}

		if name == "integrity" {
			return fmt.Errorf("assets: the integrity attribute comes from the file")
		}

		if slices.Contains(reserved, name) {
			return fmt.Errorf("assets: the %s attribute cannot be set", name)
		}

		if name == "crossorigin" {
			crossOrigin = true
		}
	}

	if integrity != "" {
		out.WriteString(` integrity="`)
		out.WriteString(template.HTMLEscapeString(integrity))
		out.WriteString(`"`)

		if !crossOrigin {
			out.WriteString(` crossorigin="anonymous"`)
		}
	}

	for i := 0; i < len(attrs); i += 2 {
		out.WriteString(" ")
		out.WriteString(strings.ToLower(attrs[i]))

		if value := attrs[i+1]; value != "" {
			out.WriteString(`="`)
			out.WriteString(template.HTMLEscapeString(value))
			out.WriteString(`"`)
		}
	}

	return nil
}

// Stylesheet returns a <link rel="stylesheet"> tag for file at its
// versioned URL, with its Subresource Integrity, as StylesheetTag writes
// it. While the manifest reloads (Reload) the tag has no integrity, as
// TagIntegrity explains. An unknown file is an error.
func (m *Manifest) Stylesheet(file string, attrs ...string) (template.HTML, error) {
	if !m.Has(file) {
		return "", fmt.Errorf("assets: unknown file %q", file)
	}

	return StylesheetTag(m.URL(file), m.TagIntegrity(file), attrs...)
}
