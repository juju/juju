// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package csclient // import "gopkg.in/juju/charmrepo.v2-unstable/csclient"

import (
	"crypto/sha512"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6-unstable"
)

// ReadSeekCloser implements io.ReadSeeker and io.Closer.
type ReadSeekCloser interface {
	io.ReadSeeker
	io.Closer
}

// openArchive is used to turn the current charm or bundle implementations
// into readers for their corresponding archive.
// It returns the corresponding archive reader, its hex-encoded SHA384 hash
// and size.
func openArchive(entity interface{}) (r ReadSeekCloser, hash string, size int64, err error) {
	var path string
	switch entity := entity.(type) {
	case archiverTo:
		// For example: charm.CharmDir or charm.BundleDir.
		file, err := newRemoveOnCloseTempFile("entity-archive")
		if err != nil {
			return nil, "", 0, errgo.Notef(err, "cannot make temporary file")
		}
		if err := entity.ArchiveTo(file); err != nil {
			file.Close()
			return nil, "", 0, errgo.Notef(err, "cannot create entity archive")
		}
		if _, err := file.Seek(0, 0); err != nil {
			file.Close()
			return nil, "", 0, errgo.Notef(err, "cannot seek")
		}
		hash, size, err = readerHashAndSize(file)
		if err != nil {
			file.Close()
			return nil, "", 0, errgo.Mask(err)
		}
		return file, hash, size, nil
	case *charm.BundleArchive:
		path = entity.Path
	case *charm.CharmArchive:
		path = entity.Path
	default:
		return nil, "", 0, errgo.Newf("cannot get the archive for entity type %T", entity)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, "", 0, errgo.Mask(err)
	}
	hash, size, err = readerHashAndSize(file)
	if err != nil {
		file.Close()
		return nil, "", 0, errgo.Mask(err)
	}
	return file, hash, size, nil
}

// readerHashAndSize returns the hex-encoded SHA384 hash and size of
// the data included in the given reader.
func readerHashAndSize(r io.ReadSeeker) (hash string, size int64, err error) {
	h := sha512.New384()
	size, err = io.Copy(h, r)
	if err != nil {
		return "", 0, errgo.Notef(err, "cannot calculate hash")
	}
	if _, err := r.Seek(0, 0); err != nil {
		return "", 0, errgo.Notef(err, "cannot seek")
	}
	return fmt.Sprintf("%x", h.Sum(nil)), size, nil
}

type archiverTo interface {
	ArchiveTo(io.Writer) error
}

// newRemoveOnCloseTempFile creates a new temporary file in the default
// directory for temporary files with a name beginning with prefix.
// The resulting file is removed when the file is closed.
func newRemoveOnCloseTempFile(prefix string) (*removeOnCloseFile, error) {
	file, err := ioutil.TempFile("", prefix)
	if err != nil {
		return nil, err
	}
	return &removeOnCloseFile{file}, nil
}

// removeOnCloseFile represents a file which is removed when closed.
type removeOnCloseFile struct {
	*os.File
}

func (r *removeOnCloseFile) Close() error {
	r.File.Close()
	return os.Remove(r.File.Name())
}
