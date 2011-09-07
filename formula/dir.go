package formula

import (
	"os"
	"path/filepath"
)

// ReadDir returns a Dir representing an expanded formula directory.
func ReadDir(path string) (dir *Dir, err os.Error) {
	meta, err := ReadMeta(filepath.Join(path, "metadata.yaml"))
	if err != nil {
		return nil, err
	}
	config, err := ReadConfig(filepath.Join(path, "config.yaml"))
	if err != nil {
		return nil, err
	}
	return &Dir{path, meta, config}, nil
}

// The Dir type encapsulates access to data and operations
// on a formula directory.
type Dir struct {
	path string
	meta *Meta
	config *Config
}

// Path returns the directory the formula is expanded under.
func (dir *Dir) Path() string {
	return dir.path
}

// Meta returns the Meta representing the metadata.yaml file
// for the formula expanded in dir.
func (dir *Dir) Meta() *Meta {
	return dir.meta
}

// Config returns the Config representing the config.yaml file
// for the formula expanded in dir.
func (dir *Dir) Config() *Config {
	return dir.config
}

// IsExpanded returns true since Dir represents an expanded formula
// directory. It will return false for a formula Bundle.
// This is useful mainly when using a formula through the
// generic Formula interface
func (dir *Dir) IsExpanded() bool {
	return true
}

// Trick to ensure Dir implements the Formula interface.
var _ Formula = (*Dir)(nil)
