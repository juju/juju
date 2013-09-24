// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"os"
	"time"
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
	return &TarFile{
		Header: tar.Header{
			Typeflag:   ftype,
			Name:       name,
			Size:       int64(len(contents)),
			Mode:       int64(mode & 0777),
			ModTime:    time.Now(),
			AccessTime: time.Now(),
			ChangeTime: time.Now(),
			Uname:      "ubuntu",
			Gname:      "ubuntu",
		},
		Contents: contents,
	}
}

// TarGz returns the given files in gzipped tar-archive format, along with the tarball size and sha256 checksum.
func TarGz(files ...*TarFile) (data []byte, size int64, checksum string) {
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
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
	data = buf.Bytes()
	size = int64(len(data))
	sha256hash := sha256.New()
	sha256hash.Write(data)
	checksum = fmt.Sprintf("%x", sha256hash.Sum(nil))
	return data, size, checksum
}
