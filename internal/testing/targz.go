// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
)

// TarFile represents a file to be archived.
type TarFile struct {
	Header   tar.Header
	Contents string
}

var modes = map[os.FileMode]byte{
	os.ModeDir:     tar.TypeDir,
	os.ModeSymlink: tar.TypeSymlink,
	0:              tar.TypeReg,
}

// NewTarFile returns a new TarFile instance with the given file
// mode and contents.
func NewTarFile(name string, mode os.FileMode, contents string) *TarFile {
	ftype := modes[mode&os.ModeType]
	if ftype == 0 {
		panic(fmt.Errorf("unexpected mode %v", mode))
	}
	// NOTE: Do not set attributes (e.g. times) dynamically, as various
	// tests expect the contents of fake tools archives to be unchanging.
	return &TarFile{
		Header: tar.Header{
			Typeflag: ftype,
			Name:     name,
			Size:     int64(len(contents)),
			Mode:     int64(mode & 0777),
			Uname:    "ubuntu",
			Gname:    "ubuntu",
		},
		Contents: contents,
	}
}

// TarGz returns the given files in gzipped tar-archive format, along with the sha256 checksum.
func TarGz(files ...*TarFile) ([]byte, string) {
	var buf bytes.Buffer
	sha256hash := sha256.New()
	gzw := gzip.NewWriter(io.MultiWriter(&buf, sha256hash))
	tarw := tar.NewWriter(gzw)

	for _, f := range files {
		err := tarw.WriteHeader(&f.Header)
		if err != nil {
			panic(err)
		}
		_, err = tarw.Write([]byte(f.Contents))
		if err != nil {
			panic(err)
		}
	}
	err := tarw.Close()
	if err != nil {
		panic(err)
	}
	err = gzw.Close()
	if err != nil {
		panic(err)
	}
	checksum := fmt.Sprintf("%x", sha256hash.Sum(nil))
	return buf.Bytes(), checksum
}
