package charm

import (
	"archive/zip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
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
	if file, err = os.Open(dir.join("revision")); err == nil {
		_, err = fmt.Fscan(file, &dir.revision)
		file.Close()
		if err != nil {
			return nil, os.NewError("invalid revision file")
		}
	} else {
		dir.revision = dir.meta.OldRevision
	}
	return dir, nil
}

// The Dir type encapsulates access to data and operations
// on a charm directory.
type Dir struct {
	Path     string
	meta     *Meta
	config   *Config
	revision int
}

// Trick to ensure *Dir implements the Charm interface.
var _ Charm = (*Dir)(nil)

// join builds a path rooted at the charm's expanded directory
// path and the extra path components provided.
func (dir *Dir) join(parts ...string) string {
	parts = append([]string{dir.Path}, parts...)
	return filepath.Join(parts...)
}

// Revision returns the revision number for the charm
// expanded in dir.
func (dir *Dir) Revision() int {
	return dir.revision
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

// SetRevision changes the charm revision number. This affects
// the revision reported by Revision and the revision of the
// charm bundled by BundleTo.
// The revision file in the charm directory is not modified.
func (dir *Dir) SetRevision(revision int) {
	dir.revision = revision
}

// SetDiskRevision does the same as SetRevision but also changes
// the revision file in the charm directory.
func (dir *Dir) SetDiskRevision(revision int) os.Error {
	dir.SetRevision(revision)
	file, err := os.OpenFile(dir.join("revision"), os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	_, err = file.Write([]byte(strconv.Itoa(revision)))
	file.Close()
	return err
}

func (dir *Dir) BundleTo(w io.Writer) (err os.Error) {
	zipw := zip.NewWriter(w)
	defer zipw.Close()
	zp := zipPacker{zipw, dir.Path}
	zp.AddRevision(dir.revision)
	return filepath.Walk(dir.Path, zp.WalkFunc())
}

type zipPacker struct {
	*zip.Writer
	root string
}

func (zp *zipPacker) WalkFunc() filepath.WalkFunc {
	return func(path string, fi *os.FileInfo, err os.Error) os.Error {
		return zp.visit(path, fi, err)
	}
}

func (zp *zipPacker) AddRevision(revision int) os.Error {
	h := &zip.FileHeader{Name: "revision"}
	h.SetMode(syscall.S_IFREG | 0644)
	w, err := zp.CreateHeader(h)
	if err == nil {
		_, err = w.Write([]byte(strconv.Itoa(revision)))
	}
	return err
}

func (zp *zipPacker) visit(path string, fi *os.FileInfo, err os.Error) os.Error {
	if err != nil {
		return err
	}
	relpath, err := filepath.Rel(zp.root, path)
	if err != nil {
		return err
	}
	method := zip.Deflate
	hidden := len(relpath) > 1 && relpath[0] == '.'
	if fi.IsDirectory() {
		if relpath == "build" {
			return filepath.SkipDir
		}
		if hidden {
			return filepath.SkipDir
		}
		relpath += "/"
		method = zip.Store
	}
	if hidden || relpath == "revision" {
		return nil
	}
	h := &zip.FileHeader{
		Name:   relpath,
		Method: method,
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
