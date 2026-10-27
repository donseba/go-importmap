package importmap

import (
	"path"
	"regexp"
	"strings"
)

// notLoaded matches the names of files no script loads: licence texts.
var notLoaded = regexp.MustCompile(`(?i)^(licen[cs]es?|copying|notice)([.-].*)?$`)

// versionedFiles are the files of the manifest whose plain URLs the map
// points at their versioned ones (WithVersionedPaths).
func (im *ImportMap) versionedFiles() []string {
	var include, exclude []string

	for _, pattern := range im.versioned {
		if rest, ok := strings.CutPrefix(pattern, "!"); ok {
			exclude = append(exclude, strings.TrimPrefix(rest, "/"))

			continue
		}

		include = append(include, strings.TrimPrefix(pattern, "/"))
	}

	everything := im.versionAll || len(include) == 0

	var files []string

	for _, file := range im.manifest.Files() {
		selected := everything && loadable(file)

		if !selected {
			selected = matchesAny(include, file)
		}

		if selected && !matchesAny(exclude, file) {
			files = append(files, file)
		}
	}

	return files
}

// loadable reports whether a script may load file: it is not the lock
// file or a licence text.
func loadable(file string) bool {
	name := path.Base(file)

	return name != LockFile && !notLoaded.MatchString(name)
}

// matchesAny reports whether file matches one of patterns.
func matchesAny(patterns []string, file string) bool {
	for _, pattern := range patterns {
		if matchPath(pattern, file) {
			return true
		}
	}

	return false
}

// matchPath reports whether file, a slash-separated path, matches pattern:
// path.Match per directory, where a "**" element matches any number of
// directories. A pattern that is the file's name matches it.
func matchPath(pattern, file string) bool {
	if pattern == file {
		return true
	}

	return matchElements(strings.Split(pattern, "/"), strings.Split(file, "/"))
}

// matchElements matches the elements of a path against those of a
// pattern.
func matchElements(pattern, elements []string) bool {
	if len(pattern) == 0 {
		return len(elements) == 0
	}

	if pattern[0] == "**" {
		for skip := 0; skip <= len(elements); skip++ {
			if matchElements(pattern[1:], elements[skip:]) {
				return true
			}
		}

		return false
	}

	if len(elements) == 0 {
		return false
	}

	matched, err := path.Match(pattern[0], elements[0])
	if err != nil || !matched {
		return false
	}

	return matchElements(pattern[1:], elements[1:])
}
