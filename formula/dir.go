package formula

import (
	"archive/zip"
	"io/ioutil"
	"os"
	"path/filepath"
)

// ReadDir returns a Dir representing an expanded formula directory.
func ReadDir(path string) (dir *Dir, err os.Error) {
	dir = &Dir{path: path}
	file, err := os.Open(dir.join("metadata.yaml"))
	if err != nil {
		return nil, err
	}
	dir.meta, err = ReadMeta(file)
	file.Close()
	if err != nil {
		return nil, err
	}
	dir.config, err = ReadConfig(dir.join("config.yaml"))
	if err != nil {
		return nil, err
	}
	return dir, nil
}

// The Dir type encapsulates access to data and operations
// on a formula directory.
type Dir struct {
	path   string
	meta   *Meta
	config *Config
}

// Trick to ensure Dir implements the Formula interface.
var _ Formula = (*Dir)(nil)

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

// join builds a path rooted at the formula's expended directory
// path and the extra path components provided.
func (dir *Dir) join(parts ...string) string {
	parts = append([]string{dir.path}, parts...)
	return filepath.Join(parts...)
}

// BundleTo builds a formula file from the expanded formula in dir.
func (dir *Dir) BundleTo(path string) (err os.Error) {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	zipw := zip.NewWriter(file)
	defer func() {
		zipw.Close()
		file.Close()
		handleZipError(&err)
		if err != nil {
			os.Remove(path)
		}
	}()
	visitor := zipVisitor{zipw, dir.path}
	walk(dir.path, &visitor)
	return nil
}

type zipVisitor struct {
	*zip.Writer
	root string
}

func (zipw *zipVisitor) VisitDir(path string, f *os.FileInfo) bool {
	relpath, err := filepath.Rel(path, zipw.root)
	zipw.Error(path, err)
	return relpath != "build"
}

func (zipw *zipVisitor) VisitFile(path string, f *os.FileInfo) {
	relpath, err := filepath.Rel(path, zipw.root)
	zipw.Error(path, err)
	w, err := zipw.Create(relpath)
	zipw.Error(path, err)
	data, err := ioutil.ReadFile(path)
	zipw.Error(path, err)
	_, err = w.Write(data)
	zipw.Error(path, err)
}

type zipError os.Error

func (zipw *zipVisitor) Error(path string, err os.Error) {
	if err != nil {
		panic(zipError(err))
	}
}

func handleZipError(err *os.Error) {
	if *err != nil {
		return // Do not override a previous problem
	}
	panicv := recover()
	if panicv == nil {
		return
	}
	if e, ok := panicv.(zipError); ok {
		*err = (os.Error)(e)
		return
	}
	panic(panicv) // Something else
}
