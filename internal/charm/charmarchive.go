// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	ziputil "github.com/juju/utils/v4/zip"
)

// CharmArchive type encapsulates access to data and operations
// on a charm archive.
type CharmArchive struct {
	zopen zipOpener

	Path string // May be empty if CharmArchive wasn't read from a file
	*charmBase
}

// Trick to ensure *CharmArchive implements the Charm interface.
var _ Charm = (*CharmArchive)(nil)

// ReadCharmArchive returns a CharmArchive for the charm in path.
func ReadCharmArchive(path string) (*CharmArchive, error) {
	a, err := readCharmArchive(newZipOpenerFromPath(path))
	if err != nil {
		return nil, err
	}
	a.Path = path
	return a, nil
}

// ReadCharmArchiveBytes returns a CharmArchive read from the given data.
// Make sure the archive fits in memory before using this.
func ReadCharmArchiveBytes(data []byte) (archive *CharmArchive, err error) {
	zopener := newZipOpenerFromReader(bytes.NewReader(data), int64(len(data)))
	return readCharmArchive(zopener)
}

// ReadCharmArchiveFromReader returns a CharmArchive that uses
// r to read the charm. The given size must hold the number
// of available bytes in the file.
//
// Note that the caller is responsible for closing r - methods on
// the returned CharmArchive may fail after that.
func ReadCharmArchiveFromReader(r io.ReaderAt, size int64) (archive *CharmArchive, err error) {
	return readCharmArchive(newZipOpenerFromReader(r, size))
}

func readCharmArchive(zopen zipOpener) (archive *CharmArchive, err error) {
	b := &CharmArchive{
		zopen:     zopen,
		charmBase: &charmBase{},
	}
	zipr, err := zopen.openZip()
	if err != nil {
		return nil, err
	}
	defer func() { _ = zipr.Close() }()
	reader, err := zipOpenFile(zipr, "metadata.yaml")
	if err != nil {
		return nil, err
	}
	b.meta, err = ReadMeta(reader)
	_ = reader.Close()
	if err != nil {
		return nil, err
	}

	// Try to read the optional manifest.yaml, it's required to determine if
	// this charm is v1 or not.
	reader, err = zipOpenFile(zipr, "manifest.yaml")
	if _, ok := err.(*noCharmArchiveFile); ok {
		b.manifest = nil
	} else if err != nil {
		return nil, errors.Annotatef(err, `opening "manifest.yaml" file`)
	} else {
		b.manifest, err = ReadManifest(reader)
		_ = reader.Close()
		if err != nil {
			return nil, errors.Annotatef(err, `parsing "manifest.yaml" file`)
		}
	}

	reader, err = zipOpenFile(zipr, "config.yaml")
	if _, ok := err.(*noCharmArchiveFile); ok {
		b.config = NewConfig()
	} else if err != nil {
		return nil, err
	} else {
		b.config, err = ReadConfig(reader)
		_ = reader.Close()
		if err != nil {
			return nil, err
		}
	}

	if b.actions, err = getActions(
		b.meta.Name,
		func(file string) (io.ReadCloser, error) {
			return zipOpenFile(zipr, file)
		},
		func(err error) bool {
			_, ok := err.(*noCharmArchiveFile)
			return ok
		},
	); err != nil {
		return nil, err
	}

	reader, err = zipOpenFile(zipr, "revision")
	if err != nil {
		if _, ok := err.(*noCharmArchiveFile); !ok {
			return nil, err
		}
	} else {
		_, err = fmt.Fscan(reader, &b.revision)
		if err != nil {
			return nil, errors.New("invalid revision file")
		}
	}

	reader, err = zipOpenFile(zipr, "lxd-profile.yaml")
	if _, ok := err.(*noCharmArchiveFile); ok {
		b.lxdProfile = NewLXDProfile()
	} else if err != nil {
		return nil, err
	} else {
		b.lxdProfile, err = ReadLXDProfile(reader)
		_ = reader.Close()
		if err != nil {
			return nil, err
		}
	}

	reader, err = zipOpenFile(zipr, "version")
	if err != nil {
		if _, ok := err.(*noCharmArchiveFile); !ok {
			return nil, err
		}
	} else {
		b.version, err = ReadVersion(reader)
		_ = reader.Close()
		if err != nil {
			return nil, err
		}
	}

	return b, nil
}

type fileOpener func(string) (io.ReadCloser, error)

func getActions(charmName string, open fileOpener, isNotFound func(error) bool) (actions *Actions, err error) {
	reader, err := open("actions.yaml")
	if err == nil {
		defer reader.Close()
		return ReadActionsYaml(charmName, reader)
	} else if !isNotFound(err) {
		return nil, err
	}
	return NewActions(), nil
}

func zipOpenFile(zipr *zipReadCloser, path string) (rc io.ReadCloser, err error) {
	for _, fh := range zipr.File {
		if fh.Name == path {
			return fh.Open()
		}
	}
	return nil, &noCharmArchiveFile{path}
}

type noCharmArchiveFile struct {
	path string
}

func (err noCharmArchiveFile) Error() string {
	return fmt.Sprintf("archive file %q not found", err.path)
}

type zipReadCloser struct {
	io.Closer
	*zip.Reader
}

// zipOpener holds the information needed to open a zip
// file.
type zipOpener interface {
	openZip() (*zipReadCloser, error)
}

// newZipOpenerFromPath returns a zipOpener that can be
// used to read the archive from the given path.
func newZipOpenerFromPath(path string) zipOpener {
	return &zipPathOpener{path: path}
}

// newZipOpenerFromReader returns a zipOpener that can be
// used to read the archive from the given ReaderAt
// holding the given number of bytes.
func newZipOpenerFromReader(r io.ReaderAt, size int64) zipOpener {
	return &zipReaderOpener{
		r:    r,
		size: size,
	}
}

type zipPathOpener struct {
	path string
}

func (zo *zipPathOpener) openZip() (*zipReadCloser, error) {
	f, err := os.Open(zo.path)
	if err != nil {
		return nil, err
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	r, err := zip.NewReader(f, fi.Size())
	if err != nil {
		f.Close()
		return nil, err
	}
	return &zipReadCloser{Closer: f, Reader: r}, nil
}

type zipReaderOpener struct {
	r    io.ReaderAt
	size int64
}

func (zo *zipReaderOpener) openZip() (*zipReadCloser, error) {
	r, err := zip.NewReader(zo.r, zo.size)
	if err != nil {
		return nil, err
	}
	return &zipReadCloser{Closer: ioutil.NopCloser(nil), Reader: r}, nil
}

// ArchiveMembers returns a set of the charm's contents.
func (a *CharmArchive) ArchiveMembers() (set.Strings, error) {
	zipr, err := a.zopen.openZip()
	if err != nil {
		return set.NewStrings(), err
	}
	defer zipr.Close()
	paths, err := ziputil.Find(zipr.Reader, "*")
	if err != nil {
		return set.NewStrings(), err
	}
	manifest := set.NewStrings(paths...)
	// We always write out a revision file, even if there isn't one in the
	// archive; and we always strip ".", because that's sometimes not present.
	manifest.Add("revision")
	manifest.Remove(".")
	return manifest, nil
}

// ExpandTo expands the charm archive into dir, creating it if necessary.
// If any errors occur during the expansion procedure, the process will
// abort.
func (a *CharmArchive) ExpandTo(dir string) error {
	zipr, err := a.zopen.openZip()
	if err != nil {
		return err
	}
	defer zipr.Close()
	if err := ziputil.ExtractAll(zipr.Reader, dir); err != nil {
		return err
	}
	hooksDir := filepath.Join(dir, "hooks")
	fixHook := fixHookFunc(hooksDir, a.meta.Hooks())
	if err := filepath.Walk(hooksDir, fixHook); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	}
	revFile, err := os.Create(filepath.Join(dir, "revision"))
	if err != nil {
		return err
	}
	if _, err := revFile.Write([]byte(strconv.Itoa(a.revision))); err != nil {
		return err
	}
	if err := revFile.Sync(); err != nil {
		return err
	}
	if err := revFile.Close(); err != nil {
		return err
	}
	return nil
}

// fixHookFunc returns a WalkFunc that makes sure hooks are owner-executable.
func fixHookFunc(hooksDir string, hookNames map[string]bool) filepath.WalkFunc {
	return func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		mode := info.Mode()
		if path != hooksDir && mode.IsDir() {
			return filepath.SkipDir
		}
		if name := filepath.Base(path); hookNames[name] {
			if mode&0100 == 0 {
				return os.Chmod(path, mode|0100)
			}
		}
		return nil
	}
}
