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

	"launchpad.net/juju-core/utils/set"
	ziputil "launchpad.net/juju-core/utils/zip"
)

// The Bundle type encapsulates access to data and operations
// on a charm bundle.
type Bundle struct {
	Path     string // May be empty if Bundle wasn't read from a file
	meta     *Meta
	config   *Config
	revision int
	r        io.ReaderAt
	size     int64
}

// Trick to ensure *Bundle implements the Charm interface.
var _ Charm = (*Bundle)(nil)

// ReadBundle returns a Bundle for the charm in path.
func ReadBundle(path string) (bundle *Bundle, err error) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return
	}
	b, err := readBundle(f, fi.Size())
	if err != nil {
		return
	}
	b.Path = path
	return b, nil
}

// ReadBundleBytes returns a Bundle read from the given data.
// Make sure the bundle fits in memory before using this.
func ReadBundleBytes(data []byte) (bundle *Bundle, err error) {
	return readBundle(readAtBytes(data), int64(len(data)))
}

func readBundle(r io.ReaderAt, size int64) (bundle *Bundle, err error) {
	b := &Bundle{r: r, size: size}
	zipr, err := zip.NewReader(r, size)
	if err != nil {
		return
	}
	reader, err := zipOpen(zipr, "metadata.yaml")
	if err != nil {
		return
	}
	b.meta, err = ReadMeta(reader)
	reader.Close()
	if err != nil {
		return
	}

	reader, err = zipOpen(zipr, "config.yaml")
	if _, ok := err.(*noBundleFile); ok {
		b.config = NewConfig()
	} else if err != nil {
		return nil, err
	} else {
		b.config, err = ReadConfig(reader)
		reader.Close()
		if err != nil {
			return nil, err
		}
	}

	reader, err = zipOpen(zipr, "revision")
	if err != nil {
		if _, ok := err.(*noBundleFile); !ok {
			return
		}
		b.revision = b.meta.OldRevision
	} else {
		_, err = fmt.Fscan(reader, &b.revision)
		if err != nil {
			return nil, errors.New("invalid revision file")
		}
	}

	return b, nil
}

func zipOpen(zipr *zip.Reader, path string) (rc io.ReadCloser, err error) {
	for _, fh := range zipr.File {
		if fh.Name == path {
			return fh.Open()
		}
	}
	return nil, &noBundleFile{path}
}

type noBundleFile struct {
	path string
}

func (err noBundleFile) Error() string {
	return fmt.Sprintf("bundle file not found: %s", err.path)
}

// Revision returns the revision number for the charm
// expanded in dir.
func (b *Bundle) Revision() int {
	return b.revision
}

// SetRevision changes the charm revision number. This affects the
// revision reported by Revision and the revision of the charm
// directory created by ExpandTo.
func (b *Bundle) SetRevision(revision int) {
	b.revision = revision
}

// Meta returns the Meta representing the metadata.yaml file from bundle.
func (b *Bundle) Meta() *Meta {
	return b.meta
}

// Config returns the Config representing the config.yaml file
// for the charm bundle.
func (b *Bundle) Config() *Config {
	return b.config
}

type zipReadCloser struct {
	io.Closer
	*zip.Reader
}

// zipOpen returns a zipReadCloser.
func (b *Bundle) zipOpen() (*zipReadCloser, error) {
	// If we don't have a Path, try to use the original ReaderAt.
	if b.Path == "" {
		r, err := zip.NewReader(b.r, b.size)
		if err != nil {
			return nil, err
		}
		return &zipReadCloser{Closer: ioutil.NopCloser(nil), Reader: r}, nil
	}
	f, err := os.Open(b.Path)
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

// Manifest returns a set of the charm's contents.
func (b *Bundle) Manifest() (set.Strings, error) {
	zipr, err := b.zipOpen()
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
	// bundle; and we always strip ".", because that's sometimes not present.
	manifest.Add("revision")
	manifest.Remove(".")
	return manifest, nil
}

// ExpandTo expands the charm bundle into dir, creating it if necessary.
// If any errors occur during the expansion procedure, the process will
// abort.
func (b *Bundle) ExpandTo(dir string) (err error) {
	zipr, err := b.zipOpen()
	if err != nil {
		return err
	}
	defer zipr.Close()
	if err := ziputil.ExtractAll(zipr.Reader, dir); err != nil {
		return err
	}
	hooksDir := filepath.Join(dir, "hooks")
	fixHook := fixHookFunc(hooksDir, b.meta.Hooks())
	if err := filepath.Walk(hooksDir, fixHook); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	}
	revFile, err := os.Create(filepath.Join(dir, "revision"))
	if err != nil {
		return err
	}
	_, err = revFile.Write([]byte(strconv.Itoa(b.revision)))
	revFile.Close()
	return err
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

// FWIW, being able to do this is awesome.
type readAtBytes []byte

func (b readAtBytes) ReadAt(out []byte, off int64) (n int, err error) {
	return copy(out, b[off:]), nil
}
