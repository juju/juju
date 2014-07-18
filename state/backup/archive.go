// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

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

	return
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

// tarFiles creates a tar archive at targetPath holding the files listed
// in fileList. If compress is true, the archive will also be gzip
// compressed.
func tarFiles(fileList []string, targetPath, strip string, compress bool) (shaSum string, err error) {
	shahash := sha1.New()
	if err := tarAndHashFiles(fileList, targetPath, strip, compress, shahash); err != nil {
		return "", err
	}
	// we use a base64 encoded sha1 hash, because this is the hash
	// used by RFC 3230 Digest headers in http responses
	encodedHash := base64.StdEncoding.EncodeToString(shahash.Sum(nil))
	return encodedHash, nil
}

func tarAndHashFiles(fileList []string, targetPath, strip string, compress bool, hashw io.Writer) (err error) {
	checkClose := func(w io.Closer) {
		if closeErr := w.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("error closing backup file: %v", closeErr)
		}
	}

	// Create the file.
	f, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("cannot create backup file %q", targetPath)
	}
	defer checkClose(f)

	// Set it to hash the file.
	w := io.MultiWriter(f, hashw)

	// Write out the archive.
	ar := archive{fileList, strip}
	if compress {
		return ar.WriteGzipped(w)
	} else {
		return ar.Write(w)
	}
}
