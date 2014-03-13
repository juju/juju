// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"launchpad.net/juju-core/log"
)

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

// ReadDir returns a Dir representing an expanded charm directory.
func ReadDir(path string) (dir *Dir, err error) {
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
	if _, ok := err.(*os.PathError); ok {
		dir.config = NewConfig()
	} else if err != nil {
		return nil, err
	} else {
		dir.config, err = ReadConfig(file)
		file.Close()
		if err != nil {
			return nil, err
		}
	}
	if file, err = os.Open(dir.join("revision")); err == nil {
		_, err = fmt.Fscan(file, &dir.revision)
		file.Close()
		if err != nil {
			return nil, errors.New("invalid revision file")
		}
	} else {
		dir.revision = dir.meta.OldRevision
	}

	return dir, nil
}

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
func (dir *Dir) SetDiskRevision(revision int) error {
	dir.SetRevision(revision)
	file, err := os.OpenFile(dir.join("revision"), os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	_, err = file.Write([]byte(strconv.Itoa(revision)))
	file.Close()
	return err
}

// BundleTo creates a charm file from the charm expanded in dir.
// By convention a charm bundle should have a ".charm" suffix.
func (dir *Dir) BundleTo(w io.Writer) (err error) {
	zipw := zip.NewWriter(w)
	defer zipw.Close()
	zp := zipPacker{zipw, dir.Path, dir.Meta().Hooks()}
	zp.AddRevision(dir.revision)
	return filepath.Walk(dir.Path, zp.WalkFunc())
}

type zipPacker struct {
	*zip.Writer
	root  string
	hooks map[string]bool
}

func (zp *zipPacker) WalkFunc() filepath.WalkFunc {
	return func(path string, fi os.FileInfo, err error) error {
		return zp.visit(path, fi, err)
	}
}

func (zp *zipPacker) AddRevision(revision int) error {
	h := &zip.FileHeader{Name: "revision"}
	h.SetMode(syscall.S_IFREG | 0644)
	w, err := zp.CreateHeader(h)
	if err == nil {
		_, err = w.Write([]byte(strconv.Itoa(revision)))
	}
	return err
}

func (zp *zipPacker) visit(path string, fi os.FileInfo, err error) error {
	if err != nil {
		return err
	}
	relpath, err := filepath.Rel(zp.root, path)
	if err != nil {
		return err
	}
	method := zip.Deflate
	hidden := len(relpath) > 1 && relpath[0] == '.'
	if fi.IsDir() {
		if relpath == "build" {
			return filepath.SkipDir
		}
		if hidden {
			return filepath.SkipDir
		}
		relpath += "/"
		method = zip.Store
	}

	mode := fi.Mode()
	if err := checkFileType(relpath, mode); err != nil {
		return err
	}
	if mode&os.ModeSymlink != 0 {
		method = zip.Store
	}
	if hidden || relpath == "revision" {
		return nil
	}
	h := &zip.FileHeader{
		Name:   relpath,
		Method: method,
	}

	perm := os.FileMode(0644)
	if mode&os.ModeSymlink != 0 {
		perm = 0777
	} else if mode&0100 != 0 {
		perm = 0755
	}
	if filepath.Dir(relpath) == "hooks" {
		hookName := filepath.Base(relpath)
		if _, ok := zp.hooks[hookName]; !fi.IsDir() && ok && mode&0100 == 0 {
			log.Warningf("charm: making %q executable in charm", path)
			perm = perm | 0100
		}
	}
	h.SetMode(mode&^0777 | perm)

	w, err := zp.CreateHeader(h)
	if err != nil || fi.IsDir() {
		return err
	}
	var data []byte
	if mode&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			return err
		}
		if err := checkSymlinkTarget(zp.root, relpath, target); err != nil {
			return err
		}
		data = []byte(target)
	} else {
		data, err = ioutil.ReadFile(path)
		if err != nil {
			return err
		}
	}
	_, err = w.Write(data)
	return err
}

func checkSymlinkTarget(basedir, symlink, target string) error {
	if filepath.IsAbs(target) {
		return fmt.Errorf("symlink %q is absolute: %q", symlink, target)
	}
	p := filepath.Join(filepath.Dir(symlink), target)
	if p == ".." || strings.HasPrefix(p, "../") {
		return fmt.Errorf("symlink %q links out of charm: %q", symlink, target)
	}
	return nil
}

func checkFileType(path string, mode os.FileMode) error {
	e := "file has an unknown type: %q"
	switch mode & os.ModeType {
	case os.ModeDir, os.ModeSymlink, 0:
		return nil
	case os.ModeNamedPipe:
		e = "file is a named pipe: %q"
	case os.ModeSocket:
		e = "file is a socket: %q"
	case os.ModeDevice:
		e = "file is a device: %q"
	}
	return fmt.Errorf(e, path)
}
