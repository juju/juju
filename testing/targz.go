// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
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

// TarGz returns the given files in gzipped tar-archive format.
func TarGz(files ...*TarFile) []byte {
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
	return buf.Bytes()
}
