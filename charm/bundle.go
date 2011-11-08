package charm

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
)

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
	b, err := readBundle(f, fi.Size)
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
	if err != nil {
		return
	}
	b.config, err = ReadConfig(reader)
	reader.Close()
	if err != nil {
		return
	}
	return b, nil
}

func zipOpen(zipr *zip.Reader, path string) (rc io.ReadCloser, err error) {
	for _, fh := range zipr.File {
		if fh.Name == path {
			return fh.Open()
		}
	}
	return nil, errorf("bundle file not found: %s", path)
}

// The Bundle type encapsulates access to data and operations
// on a charm bundle.
type Bundle struct {
	Path   string // May be empty if Bundle wasn't read from a file
	meta   *Meta
	config *Config
	r      io.ReaderAt
	size   int64
}

// Trick to ensure *Bundle implements the Charm interface.
var _ Charm = (*Bundle)(nil)

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
		size = fi.Size
	}

	zipr, err := zip.NewReader(r, size)
	if err != nil {
		return err
	}

	var lasterr error
	for _, zfile := range zipr.File {
		if err := b.expand(dir, zfile); err != nil {
			lasterr = err
		}
	}
	return lasterr
}

func (b *Bundle) expand(dir string, zfile *zip.File) error {
	r, err := zfile.Open()
	if err != nil {
		return err
	}
	defer r.Close()

	path := filepath.Join(dir, filepath.Clean(zfile.Name))
	if strings.HasSuffix(zfile.Name, "/") {
		err = os.MkdirAll(path, 0755)
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
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, r)
	f.Close()
	return err
}

// FWIW, being able to do this is awesome.
type readAtBytes []byte

func (b readAtBytes) ReadAt(out []byte, off int64) (n int, err error) {
	return copy(out, b[off:]), nil
}
