// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm

import (
	"bytes"
	"io"
	"io/ioutil"

	ziputil "github.com/juju/utils/zip"
)

type BundleArchive struct {
	zopen zipOpener

	Path   string
	data   *BundleData
	readMe string
}

// ReadBundleArchive reads a bundle archive from the given file path.
func ReadBundleArchive(path string) (*BundleArchive, error) {
	a, err := readBundleArchive(newZipOpenerFromPath(path))
	if err != nil {
		return nil, err
	}
	a.Path = path
	return a, nil
}

// ReadBundleArchiveBytes reads a bundle archive from the given byte
// slice.
func ReadBundleArchiveBytes(data []byte) (*BundleArchive, error) {
	zopener := newZipOpenerFromReader(bytes.NewReader(data), int64(len(data)))
	return readBundleArchive(zopener)
}

// ReadBundleArchiveFromReader returns a BundleArchive that uses
// r to read the bundle. The given size must hold the number
// of available bytes in the file.
//
// Note that the caller is responsible for closing r - methods on
// the returned BundleArchive may fail after that.
func ReadBundleArchiveFromReader(r io.ReaderAt, size int64) (*BundleArchive, error) {
	return readBundleArchive(newZipOpenerFromReader(r, size))
}

func readBundleArchive(zopen zipOpener) (*BundleArchive, error) {
	a := &BundleArchive{
		zopen: zopen,
	}
	zipr, err := zopen.openZip()
	if err != nil {
		return nil, err
	}
	defer zipr.Close()
	reader, err := zipOpenFile(zipr, "bundle.yaml")
	if err != nil {
		return nil, err
	}
	a.data, err = ReadBundleData(reader)
	reader.Close()
	if err != nil {
		return nil, err
	}
	reader, err = zipOpenFile(zipr, "README.md")
	if err != nil {
		return nil, err
	}
	readMe, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	a.readMe = string(readMe)
	return a, nil
}

// Data implements Bundle.Data.
func (a *BundleArchive) Data() *BundleData {
	return a.data
}

// ReadMe implements Bundle.ReadMe.
func (a *BundleArchive) ReadMe() string {
	return a.readMe
}

// ExpandTo expands the bundle archive into dir, creating it if necessary.
// If any errors occur during the expansion procedure, the process will
// abort.
func (a *BundleArchive) ExpandTo(dir string) error {
	zipr, err := a.zopen.openZip()
	if err != nil {
		return err
	}
	defer zipr.Close()
	return ziputil.ExtractAll(zipr.Reader, dir)
}
