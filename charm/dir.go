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

// join builds a path rooted at the charm's expanded directory
// path and the extra path components provided.
func (dir *Dir) join(parts ...string) string {
	parts = append([]string{dir.Path}, parts...)
	return filepath.Join(parts...)
}

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
	defer zipw.Close()
	zp := zipPacker{zipw, dir.Path}
	return filepath.Walk(dir.Path, zp.WalkFunc())
}

type zipPacker struct {
	*zip.Writer
	root string
}

func (zp *zipPacker) WalkFunc() filepath.WalkFunc {
	return func(path string, fi *os.FileInfo, err os.Error) os.Error {
		return zp.Visit(path, fi, err)
	}
}

func (zp *zipPacker) Visit(path string, fi *os.FileInfo, err os.Error) os.Error {
	if err != nil {
		return err
	}
	relpath, err := filepath.Rel(zp.root, path)
	if err != nil {
		return err
	}
	hidden := len(relpath) > 1 && relpath[0] == '.'
	if fi.IsDirectory() {
		if relpath == "build" {
			return filepath.SkipDir
		}
		if hidden {
			return filepath.SkipDir
		}
		relpath += "/"
	}
	if hidden {
		return nil
	}
	h := &zip.FileHeader{
		Name: relpath,
		Method: zip.Deflate,
	}
	h.SetMode(fi.Mode)
	w, err := zp.CreateHeader(h)
	if err != nil || fi.IsDirectory() {
		return err
	}
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}
