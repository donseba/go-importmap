package library

import "path/filepath"

const (
	// FileTypeJS represents a JavaScript file
	FileTypeJS FileType = "js"
	// FileTypeCSS represents a CSS file
	FileTypeCSS FileType = "css"
	// FileTypeOther represents a file that is not a JavaScript or CSS file
	FileTypeOther FileType = "other"
)

type FileType string

type Files []File

type File struct {
	Path      string
	LocalPath string
	Type      FileType
}

func ExtractFileType(filename string) FileType {
	switch filepath.Ext(filename) {
	case ".js":
		return FileTypeJS
	case ".css":
		return FileTypeCSS
	default:
		return FileTypeOther
	}
}
