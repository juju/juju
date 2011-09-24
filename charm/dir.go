package charm

import (
	"archive/zip"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
)

// ReadDir returns a Dir representing an expanded charm directory.
func ReadDir(path string) (dir *Dir, err os.Error) {
	dir = &Dir{Path: path}
	file, err := os.Open(dir.join("metadata.yaml"))
	if err != nil {
		return nil, err
	}
	dir.meta, err = ReadMeta(file)
	file.Close()
	if err != nil {
		return nil, err
	}
	file, err = os.Open(dir.join("config.yaml"))
	if err != nil {
		return nil, err
	}
	dir.config, err = ReadConfig(file)
	file.Close()
	if err != nil {
		return nil, err
	}
	return dir, nil
}

// The Dir type encapsulates access to data and operations
// on a charm directory.
type Dir struct {
	Path   string
	meta   *Meta
	config *Config
}

// Trick to ensure *Dir implements the Charm interface.
var _ Charm = (*Dir)(nil)

// Meta returns the Meta representing the metadata.yaml file
// for the charm expanded in dir.
func (dir *Dir) Meta() *Meta {
	return dir.meta
}

// Config returns the Config representing the config.yaml file
// for the charm expanded in dir.
func (dir *Dir) Config() *Config {
	return dir.config
}

// BundleTo creates a charm file from the charm expanded in dir.
func (dir *Dir) BundleTo(w io.Writer) (err os.Error) {
	zipw := zip.NewWriter(w)
	defer func() {
		zipw.Close()
		handleZipError(&err)
	}()
	visitor := zipVisitor{zipw, dir.Path}
	walk(dir.Path, &visitor)
	return nil
}

type zipVisitor struct {
	*zip.Writer
	root string
}

func (zipw *zipVisitor) VisitDir(path string, f *os.FileInfo) bool {
	relpath, err := filepath_Rel(zipw.root, path)
	zipw.Error(path, err)
	return relpath != "build"
}

func (zipw *zipVisitor) VisitFile(path string, f *os.FileInfo) {
	relpath, err := filepath_Rel(zipw.root, path)
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

// join builds a path rooted at the charm's expended directory
// path and the extra path components provided.
func (dir *Dir) join(parts ...string) string {
	parts = append([]string{dir.Path}, parts...)
	return filepath.Join(parts...)
}
