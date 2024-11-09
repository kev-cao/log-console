package pathutils

import (
	"os"
	"path/filepath"
	"strings"
)

// AbsolutePath returns the absolute path of a given path, resolving relative
// paths to the current working directory and ~ to the user's home directory.
func AbsolutePath(path string) (string, error) {
	if strings.HasPrefix(path, "~") {
		path = strings.Replace(path, "~", os.Getenv("HOME"), 1)
	}

	return filepath.Abs(path)
}
