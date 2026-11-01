package library

import "testing"

func TestExtractFileType(t *testing.T) {
	tests := []struct {
		filename string
		want     FileType
	}{
		{"htmx.min.js", FileTypeJS},
		{"dist/js/bootstrap.js", FileTypeJS},
		{"bootstrap.min.css", FileTypeCSS},
		{"htmx.esm.mjs", FileTypeOther},
		{"LICENSE", FileTypeOther},
		{"dist.js/README", FileTypeOther},
	}

	for _, tt := range tests {
		if got := ExtractFileType(tt.filename); got != tt.want {
			t.Errorf("ExtractFileType(%q) = %q, want %q", tt.filename, got, tt.want)
		}
	}
}

func TestFileNameMin(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{"htmx.js", "htmx.min.js"},
		{"dist/htmx.esm.js", "dist/htmx.esm.min.js"},
		{"bootstrap.css", "bootstrap.min.css"},
		{"htmx.mjs", "htmx.mjs"},
		{"LICENSE", "LICENSE"},
	}

	for _, tt := range tests {
		if got := FileNameMin(tt.filename); got != tt.want {
			t.Errorf("FileNameMin(%q) = %q, want %q", tt.filename, got, tt.want)
		}
	}
}
