package formula

import (
	"archive/zip"
	"io"
	"os"
)

// ReadBundle returns a Bundle for the formula in path.
func ReadBundle(path string) (bundle *Bundle, err os.Error) {
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
func ReadBundleBytes(data []byte) (bundle *Bundle, err os.Error) {
	return readBundle(readAtBytes(data), int64(len(data)))
}

func readBundle(r io.ReaderAt, size int64) (bundle *Bundle, err os.Error) {
	b := &Bundle{}
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

func zipOpen(zipr *zip.Reader, path string) (rc io.ReadCloser, err os.Error) {
	for _, fh := range zipr.File {
		if fh.Name == path {
			return fh.Open()
		}
	}
	return nil, errorf("bundle file not found: %s", path)
}

// The Bundle type encapsulates access to data and operations
// on a formula bundle.
type Bundle struct {
	Path   string // May be empty if Bundle wasn't read from a file
	meta   *Meta
	config *Config
}

// Trick to ensure *Bundle implements the Formula interface.
var _ Formula = (*Bundle)(nil)

// Meta returns the Meta representing the metadata.yaml file from bundle.
func (b *Bundle) Meta() *Meta {
	return b.meta
}

// Config returns the Config representing the config.yaml file
// for the formula bundle.
func (b *Bundle) Config() *Config {
	return b.config
}

// ExpandTo expands the formula bundle into dir, creating it if necessary.
func (b *Bundle) ExpandTo(dir string) (err os.Error) {
	return nil
}

// FWIW, being able to do this is awesome.
type readAtBytes []byte

func (b readAtBytes) ReadAt(out []byte, off int64) (n int, err os.Error) {
	return copy(out, b[off:]), nil
}

