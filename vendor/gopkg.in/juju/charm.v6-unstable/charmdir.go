// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// The CharmDir type encapsulates access to data and operations
// on a charm directory.
type CharmDir struct {
	Path     string
	meta     *Meta
	config   *Config
	metrics  *Metrics
	actions  *Actions
	revision int
}

// Trick to ensure *CharmDir implements the Charm interface.
var _ Charm = (*CharmDir)(nil)

// ReadCharmDir returns a CharmDir representing an expanded charm directory.
func ReadCharmDir(path string) (dir *CharmDir, err error) {
	dir = &CharmDir{Path: path}
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

	file, err = os.Open(dir.join("metrics.yaml"))
	if err == nil {
		dir.metrics, err = ReadMetrics(file)
		file.Close()
		if err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	file, err = os.Open(dir.join("actions.yaml"))
	if _, ok := err.(*os.PathError); ok {
		dir.actions = NewActions()
	} else if err != nil {
		return nil, err
	} else {
		dir.actions, err = ReadActionsYaml(file)
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
	}

	return dir, nil
}

// join builds a path rooted at the charm's expanded directory
// path and the extra path components provided.
func (dir *CharmDir) join(parts ...string) string {
	parts = append([]string{dir.Path}, parts...)
	return filepath.Join(parts...)
}

// Revision returns the revision number for the charm
// expanded in dir.
func (dir *CharmDir) Revision() int {
	return dir.revision
}

// Meta returns the Meta representing the metadata.yaml file
// for the charm expanded in dir.
func (dir *CharmDir) Meta() *Meta {
	return dir.meta
}

// Config returns the Config representing the config.yaml file
// for the charm expanded in dir.
func (dir *CharmDir) Config() *Config {
	return dir.config
}

// Metrics returns the Metrics representing the metrics.yaml file
// for the charm expanded in dir.
func (dir *CharmDir) Metrics() *Metrics {
	return dir.metrics
}

// Actions returns the Actions representing the actions.yaml file
// for the charm expanded in dir.
func (dir *CharmDir) Actions() *Actions {
	return dir.actions
}

// SetRevision changes the charm revision number. This affects
// the revision reported by Revision and the revision of the
// charm archived by ArchiveTo.
// The revision file in the charm directory is not modified.
func (dir *CharmDir) SetRevision(revision int) {
	dir.revision = revision
}

// SetDiskRevision does the same as SetRevision but also changes
// the revision file in the charm directory.
func (dir *CharmDir) SetDiskRevision(revision int) error {
	dir.SetRevision(revision)
	file, err := os.OpenFile(dir.join("revision"), os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	_, err = file.Write([]byte(strconv.Itoa(revision)))
	file.Close()
	return err
}

// resolveSymlinkedRoot returns the target destination of a
// charm root directory if the root directory is a symlink.
func resolveSymlinkedRoot(rootPath string) (string, error) {
	info, err := os.Lstat(rootPath)
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		rootPath, err = filepath.EvalSymlinks(rootPath)
		if err != nil {
			return "", fmt.Errorf("cannot read path symlink at %q: %v", rootPath, err)
		}
	}
	return rootPath, nil
}

// ArchiveTo creates a charm file from the charm expanded in dir.
// By convention a charm archive should have a ".charm" suffix.
func (dir *CharmDir) ArchiveTo(w io.Writer) error {
	return writeArchive(w, dir.Path, dir.revision, dir.Meta().Hooks())
}

func writeArchive(w io.Writer, path string, revision int, hooks map[string]bool) error {
	zipw := zip.NewWriter(w)
	defer zipw.Close()

	// The root directory may be symlinked elsewhere so
	// resolve that before creating the zip.
	rootPath, err := resolveSymlinkedRoot(path)
	if err != nil {
		return err
	}
	zp := zipPacker{zipw, rootPath, hooks}
	if revision != -1 {
		zp.AddRevision(revision)
	}
	return filepath.Walk(rootPath, zp.WalkFunc())
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
		if _, ok := zp.hooks[hookName]; ok && !fi.IsDir() && mode&0100 == 0 {
			logger.Warningf("making %q executable in charm", path)
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
		_, err = w.Write(data)
	} else {
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(w, file)
	}
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
