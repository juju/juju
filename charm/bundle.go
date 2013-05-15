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

// ExpandTo expands the charm bundle into dir, creating it if necessary.
// If any errors occur during the expansion procedure, the process will
// continue. Only the last error found is returned.
func (b *Bundle) ExpandTo(dir string) (err error) {
	// If we have a Path, reopen the file. Otherwise, try to use
	// the original ReaderAt.
	r := b.r
	size := b.size
	if b.Path != "" {
		f, err := os.Open(b.Path)
		if err != nil {
			return err
		}
		defer f.Close()
		fi, err := f.Stat()
		if err != nil {
			return err
		}
		r = f
		size = fi.Size()
	}

	zipr, err := zip.NewReader(r, size)
	if err != nil {
		return err
	}

	hooks := b.meta.Hooks()
	var lasterr error
	for _, zfile := range zipr.File {
		if err := b.expand(hooks, dir, zfile); err != nil {
			lasterr = err
		}
	}

	revFile, err := os.Create(filepath.Join(dir, "revision"))
	if err != nil {
		return err
	}
	_, err = revFile.Write([]byte(strconv.Itoa(b.revision)))
	revFile.Close()
	if err != nil {
		return err
	}
	return lasterr
}

// expand unpacks a charm's zip file into the given directory.
// The hooks map holds all the possible hook names in the
// charm.
func (b *Bundle) expand(hooks map[string]bool, dir string, zfile *zip.File) error {
	cleanName := filepath.Clean(zfile.Name)
	if cleanName == "revision" {
		return nil
	}

	r, err := zfile.Open()
	if err != nil {
		return err
	}
	defer r.Close()

	mode := zfile.Mode()
	path := filepath.Join(dir, cleanName)
	if strings.HasSuffix(zfile.Name, "/") || mode&os.ModeDir != 0 {
		err = os.MkdirAll(path, mode&0777)
		if err != nil {
			return err
		}
		return nil
	}

	base, _ := filepath.Split(path)
	err = os.MkdirAll(base, 0755)
	if err != nil {
		return err
	}

	if mode&os.ModeSymlink != 0 {
		data, err := ioutil.ReadAll(r)
		if err != nil {
			return err
		}
		target := string(data)
		if err := checkSymlinkTarget(dir, cleanName, target); err != nil {
			return err
		}
		return os.Symlink(target, path)
	}
	if filepath.Dir(cleanName) == "hooks" {
		hookName := filepath.Base(cleanName)
		if _, ok := hooks[hookName]; mode&os.ModeType == 0 && ok {
			// Set all hooks executable (by owner)
			mode = mode | 0100
		}
	}

	if err := checkFileType(cleanName, mode); err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode&0777)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, r)
	f.Close()
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

// FWIW, being able to do this is awesome.
type readAtBytes []byte

func (b readAtBytes) ReadAt(out []byte, off int64) (n int, err error) {
	return copy(out, b[off:]), nil
}
