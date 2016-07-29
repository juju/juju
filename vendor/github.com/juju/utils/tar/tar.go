// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

// This package provides convenience helpers on top of archive/tar
// to be able to tar/untar files with a functionality closer
// to gnu tar command.
package tar

import (
	"archive/tar"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/utils/symlink"
)

// FindFile returns the header and ReadCloser for the entry in the
// tarfile that matches the filename.  If nothing matches, an
// errors.NotFound error is returned.
func FindFile(tarFile io.Reader, filename string) (*tar.Header, io.Reader, error) {
	reader := tar.NewReader(tarFile)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, errors.Trace(err)
		}

		if header.Name == filename {
			return header, reader, nil
		}
	}

	return nil, nil, errors.NotFoundf(filename)
}

// TarFiles writes a tar stream into target holding the files listed
// in fileList. strip will be removed from the beginning of all the paths
// when stored (much like gnu tar -C option)
// Returns a Sha sum of the tar and nil if everything went well
// or empty sting and error in case of error.
// We use a base64 encoded sha1 hash, because this is the hash
// used by RFC 3230 Digest headers in http responses
func TarFiles(fileList []string, target io.Writer, strip string) (shaSum string, err error) {
	shahash := sha1.New()
	if err := tarAndHashFiles(fileList, target, strip, shahash); err != nil {
		return "", err
	}
	encodedHash := base64.StdEncoding.EncodeToString(shahash.Sum(nil))
	return encodedHash, nil
}

func tarAndHashFiles(fileList []string, target io.Writer, strip string, hashw io.Writer) (err error) {
	checkClose := func(w io.Closer) {
		if closeErr := w.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("error closing tar writer: %v", closeErr)
		}
	}

	w := io.MultiWriter(target, hashw)
	tarw := tar.NewWriter(w)
	defer checkClose(tarw)
	for _, ent := range fileList {
		if err := writeContents(ent, strip, tarw); err != nil {
			return fmt.Errorf("write to tar file failed: %v", err)
		}
	}
	return nil
}

// writeContents creates an entry for the given file
// or directory in the given tar archive.
func writeContents(fileName, strip string, tarw *tar.Writer) error {
	f, err := os.Open(fileName)
	if err != nil {
		return err
	}
	defer f.Close()
	fInfo, err := os.Lstat(fileName)
	if err != nil {
		return err
	}
	link := ""

	if fInfo.Mode()&os.ModeSymlink == os.ModeSymlink {
		link, err = filepath.EvalSymlinks(fileName)

		if err != nil {
			return fmt.Errorf("cannnot dereference symlink: %v", err)
		}

	}
	h, err := tar.FileInfoHeader(fInfo, link)
	if err != nil {
		return fmt.Errorf("cannot create tar header for %q: %v", fileName, err)
	}
	h.Name = filepath.ToSlash(strings.TrimPrefix(fileName, strip))
	if err := tarw.WriteHeader(h); err != nil {
		return fmt.Errorf("cannot write header for %q: %v", fileName, err)
	}
	if fInfo.Mode()&os.ModeSymlink == os.ModeSymlink {
		return nil
	}
	if !fInfo.IsDir() {
		if _, err := io.Copy(tarw, f); err != nil {
			return fmt.Errorf("failed to write %q: %v", fileName, err)
		}
		return nil
	}

	for {
		names, err := f.Readdirnames(100)
		// will return at most 100 names and if less than 100 remaining
		// next call will return io.EOF and no names
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("error reading directory %q: %v", fileName, err)
		}
		for _, name := range names {
			if err := writeContents(filepath.Join(fileName, name), strip, tarw); err != nil {
				return err
			}
		}
	}

}

func createAndFill(filePath string, mode int64, content io.Reader) error {
	fh, err := os.Create(filePath)
	defer fh.Close()
	if err != nil {
		return fmt.Errorf("some of the tar contents cannot be written to disk: %v", err)
	}
	_, err = io.Copy(fh, content)
	if err != nil {
		return fmt.Errorf("failed while reading tar contents: %v", err)
	}
	err = os.Chmod(fh.Name(), os.FileMode(mode))
	if err != nil {
		return fmt.Errorf("cannot set proper mode on file %q: %v", filePath, err)
	}
	return nil
}

// UntarFiles will extract the contents of tarFile using
// outputFolder as root
func UntarFiles(tarFile io.Reader, outputFolder string) error {
	tr := tar.NewReader(tarFile)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			// end of tar archive
			return nil
		}
		if err != nil {
			return fmt.Errorf("failed while reading tar header: %v", err)
		}
		fullPath := filepath.Join(outputFolder, hdr.Name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err = os.MkdirAll(fullPath, os.FileMode(hdr.Mode)); err != nil {
				return fmt.Errorf("cannot extract directory %q: %v", fullPath, err)
			}
		case tar.TypeSymlink:
			if err = symlink.New(hdr.Linkname, fullPath); err != nil {
				return fmt.Errorf("cannot extract symlink %q to %q: %v", hdr.Linkname, fullPath, err)
			}
			continue
		case tar.TypeReg, tar.TypeRegA:
			if err = createAndFill(fullPath, hdr.Mode, tr); err != nil {
				return fmt.Errorf("cannot extract file %q: %v", fullPath, err)
			}
		}
	}
	return nil
}
