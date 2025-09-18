// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package testing

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
)

// CharmDir encapsulates access to data and operations
// on a charm directory.
type CharmDir struct {
	meta       *charm.Meta
	config     *charm.ConfigSpec
	actions    *charm.Actions
	lxdProfile *charm.LXDProfile
	manifest   *charm.Manifest
	version    string
	revision   int

	Path string
}

// ReadCharmDir returns a CharmDir representing an expanded charm directory.
func ReadCharmDir(path string) (*CharmDir, error) {
	b := &CharmDir{
		Path: path,
	}

	var err error
	b.meta, err = charm.ReadCharmDirMetadata(path)
	if err != nil {
		return nil, err
	}

	// NOTE: Since 4.0, Juju has required charms have a manifest file.
	b.manifest, err = charm.ReadCharmDirManifest(path)
	if err != nil {
		return nil, err
	}

	b.config, err = charm.ReadCharmDirConfig(path)
	if errors.Is(err, charm.FileNotFound) {
		b.config = charm.NewConfig()
	} else if err != nil {
		return nil, err
	}

	b.actions, err = charm.ReadCharmDirActions(b.meta.Name, path)
	if errors.Is(err, charm.FileNotFound) {
		b.actions = charm.NewActions()
	} else if err != nil {
		return nil, err
	}

	b.revision, err = charm.ReadCharmDirRevision(path)
	if errors.Is(err, charm.FileNotFound) {
		b.revision = 0
	} else if err != nil {
		return nil, err
	}

	b.lxdProfile, err = charm.ReadCharmDirLXDProfile(path)
	if errors.Is(err, charm.FileNotFound) {
		b.lxdProfile = charm.NewLXDProfile()
	} else if err != nil {
		return nil, err
	}

	b.version, err = charm.ReadCharmDirVersion(path)
	if errors.Is(err, charm.FileNotFound) {
		b.version = ""
	} else if err != nil {
		return nil, err
	}

	return b, nil
}

// ArchiveTo creates a charm file from the charm expanded in dir.
// By convention a charm archive should have a ".charm" suffix.
func (dir *CharmDir) ArchiveTo(w io.Writer) error {
	return writeArchive(w, dir.Path, dir.revision, dir.version, dir.Meta().Hooks())
}

func (dir *CharmDir) ArchiveToPath(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return dir.ArchiveTo(f)
}

// Revision returns the revision number for the charm
// expanded in dir.
func (dir *CharmDir) Revision() int {
	return dir.revision
}

// Version returns the VCS version representing the version file from archive.
func (dir *CharmDir) Version() string {
	return dir.version
}

// Meta returns the Meta representing the metadata.yaml file
// for the charm expanded in dir.
func (dir *CharmDir) Meta() *charm.Meta {
	return dir.meta
}

// Config returns the Config representing the config.yaml file
// for the charm expanded in dir.
func (dir *CharmDir) Config() *charm.ConfigSpec {
	return dir.config
}

// Actions returns the Actions representing the actions.yaml file
// for the charm expanded in dir.
func (dir *CharmDir) Actions() *charm.Actions {
	return dir.actions
}

// LXDProfile returns the LXDProfile representing the lxd-profile.yaml file
// for the charm expanded in dir.
func (dir *CharmDir) LXDProfile() *charm.LXDProfile {
	return dir.lxdProfile
}

// Manifest returns the Manifest representing the manifest.yaml file
// for the charm expanded in dir.
func (dir *CharmDir) Manifest() *charm.Manifest {
	return dir.manifest
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

func writeArchive(
	w io.Writer,
	path string,
	revision int,
	versionString string,
	hooks map[string]bool,
) error {
	zipw := zip.NewWriter(w)
	defer zipw.Close()

	// The root directory may be symlinked elsewhere so
	// resolve that before creating the zip.
	rootPath, err := resolveSymlinkedRoot(path)
	if err != nil {
		return errors.Errorf("resolving symlinked root path: %w", err)
	}
	zp := zipPacker{
		Writer: zipw,
		root:   rootPath,
		hooks:  hooks,
	}
	if revision != -1 {
		err := zp.AddFile("revision", strconv.Itoa(revision))
		if err != nil {
			return errors.Errorf("adding 'revision' file: %w", err)
		}
	}
	if versionString != "" {
		err := zp.AddFile("version", versionString)
		if err != nil {
			return errors.Errorf("adding 'version' file: %w", err)
		}
	}
	if err := filepath.Walk(rootPath, zp.WalkFunc()); err != nil {
		return errors.Errorf("walking charm directory: %w", err)
	}
	return nil
}

type zipPacker struct {
	*zip.Writer
	root  string
	hooks map[string]bool
}

func (zp *zipPacker) WalkFunc() filepath.WalkFunc {
	return func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return errors.Errorf("visiting %q: %w", path, err)
		}

		return zp.visit(path, fi)
	}
}

func (zp *zipPacker) AddFile(filename string, value string) error {
	h := &zip.FileHeader{Name: filename}
	h.SetMode(syscall.S_IFREG | 0644)
	w, err := zp.CreateHeader(h)
	if err == nil {
		_, err = w.Write([]byte(value))
	}
	return err
}

func (zp *zipPacker) visit(path string, fi os.FileInfo) error {
	relpath, err := filepath.Rel(zp.root, path)
	if err != nil {
		return errors.Errorf("finding relative path for %q: %w", path, err)
	}

	// Replace any Windows path separators with "/".
	// zip file spec 4.4.17.1 says that separators are always "/" even on Windows.
	relpath = filepath.ToSlash(relpath)

	method := zip.Deflate
	if fi.IsDir() {
		relpath += "/"
		method = zip.Store
	}

	mode := fi.Mode()
	if err := checkFileType(relpath, mode); err != nil {
		return errors.Errorf("checking file type %q: %w", relpath, err)
	}
	if mode&os.ModeSymlink != 0 {
		method = zip.Store
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
			perm = perm | 0100
		}
	}
	h.SetMode(mode&^0777 | perm)

	w, err := zp.CreateHeader(h)
	if err != nil {
		return errors.Errorf("creating zip header for %q: %w", relpath, err)
	} else if fi.IsDir() {
		return nil
	}
	var data []byte
	if mode&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			return errors.Errorf("reading symlink target %q: %w", path, err)
		}
		if err := checkSymlinkTarget(relpath, target); err != nil {
			return errors.Errorf("checking symlink target %q: %w", target, err)
		}
		data = []byte(target)
		if _, err := w.Write(data); err != nil {
			return errors.Errorf("writing symlink target %q: %w", target, err)
		}
		return nil
	}

	file, err := os.Open(path)
	if err != nil {
		return errors.Errorf("opening file %q: %w", path, err)
	}
	defer file.Close()

	_, err = io.Copy(w, file)
	if err != nil {
		return errors.Errorf("copying file %q: %w", path, err)
	}
	return nil
}

func checkSymlinkTarget(symlink, target string) error {
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
