// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"
)

//---------------------------
// hashing support

type hashingWriterProxy struct {
	file   io.Writer
	hasher hash.Hash
	multiw io.Writer
}

func newSHA1Proxy(file io.Writer) *hashingWriterProxy {
	proxy := hashingWriterProxy{
		file:   file,
		hasher: sha1.New(),
	}
	return &proxy
}

func (h *hashingWriterProxy) Write(data []byte) (int, error) {
	if h.multiw == nil {
		h.multiw = io.MultiWriter(h.file, h.hasher)
	}
	return h.multiw.Write(data)
}

func (h *hashingWriterProxy) Hash() string {
	raw := h.hasher.Sum(nil)
	return base64.StdEncoding.EncodeToString(raw)
}

func (h *hashingWriterProxy) RawHash() string {
	raw := h.hasher.Sum(nil)
	return fmt.Sprintf("%x", raw)
}

//---------------------------
// archives

type archive struct {
	Files       []string
	StripPrefix string
}

// Write writes out the archive data for the files/directory-trees.
func (a *archive) Write(w io.Writer) (err error) {
	checkClose := func(w io.Closer) {
		if closeErr := w.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("error closing archive file: %v", closeErr)
		}
	}

	tarw := tar.NewWriter(w)
	defer checkClose(tarw)

	for _, ent := range a.Files {
		if err := a.writeTree(ent, tarw); err != nil {
			return fmt.Errorf("archive failed: %v", err)
		}
	}

	return nil
}

func (a *archive) WriteGzipped(w io.Writer) (err error) {
	checkClose := func(w io.Closer) {
		if closeErr := w.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("error closing archive file: %v", closeErr)
		}
	}

	gzw := gzip.NewWriter(w)
	defer checkClose(gzw)

	return a.Write(gzw)
}

func (a *archive) Create(filename string) (err error) {
	checkClose := func(w io.Closer) {
		if closeErr := w.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("error closing archive file: %v", closeErr)
		}
	}

	// Create the file.
	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("could not create archive file %q", filename)
	}
	defer checkClose(f)

	// Write out the archive.
	return a.Write(f)
}

// writeTree creates an entry for the given file
// or directory in the given tar archive.
func (a *archive) writeTree(fileName string, tarw *tar.Writer) error {
	// Open and inspect the file.
	f, err := os.Open(fileName)
	if err != nil {
		return err
	}
	defer f.Close()
	fInfo, err := f.Stat()
	if err != nil {
		return err
	}
	h, err := tar.FileInfoHeader(fInfo, "")
	if err != nil {
		return fmt.Errorf("cannot create tar header for %q: %v", fileName, err)
	}
	h.Name = filepath.ToSlash(strings.TrimPrefix(fileName, a.StripPrefix))

	// Write out the header.
	if err := tarw.WriteHeader(h); err != nil {
		return fmt.Errorf("cannot write header for %q: %v", fileName, err)
	}

	// Write out the contents.
	if fInfo.IsDir() {
		return a.writeDir(fileName, f, tarw)
	} else {
		_, err := io.Copy(tarw, f)
		if err != nil {
			return fmt.Errorf("failed to write %q: %v", fileName, err)
		}
	}
	return nil
}

func (a *archive) writeDir(dirname string, f *os.File, tarw *tar.Writer) error {
	if !strings.HasSuffix(dirname, sep) {
		dirname += sep
	}
	for {
		names, err := f.Readdirnames(100)
		if len(names) == 0 && err == io.EOF {
			break
		}

		if err != nil {
			return fmt.Errorf("error reading directory %q: %v", dirname, err)
		}
		for _, basename := range names {
			err := a.writeTree(filepath.Join(dirname, basename), tarw)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
