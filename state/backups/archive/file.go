// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package archive

import (
	"bytes"
	"compress/gzip"
	"io"
	"io/ioutil"
	"os"

	"github.com/juju/errors"
	"github.com/juju/utils/tar"

	"github.com/juju/juju/state/backups/metadata"
	"github.com/juju/juju/version"
)

var legacyVersion = version.Number{Major: 1, Minor: 20}

// ArchiveData is a wrapper around a backup archive file. It provides
// access to the data stored in the archive.
type ArchiveData struct {
	Archive
	data []byte
}

// NewArchiveData returns a new archive data wrapper for the specified file.
func NewArchiveData(filename string, data []byte) *ArchiveData {
	return &ArchiveData{
		Archive: Archive{
			Filename: filename,
		},
		data: data,
	}
}

// NewArchiveFile returns a new archive data wrapper for the provided file.
func NewArchiveFile(file io.Reader, filename string) (*ArchiveData, error) {
	gzr, err := gzip.NewReader(file)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer gzr.Close()

	data, err := ioutil.ReadAll(gzr)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return NewArchiveData(filename, data), nil
}

// Open Returns a new io.ReadCloser containing the archive's data.
func (ad *ArchiveData) Open() (io.ReadCloser, error) {
	if ad.data != nil {
		file := ioutil.NopCloser(bytes.NewBuffer(ad.data))
		return file, nil
	}

	file, err := os.Open(ad.Filename)
	if err != nil {
		return nil, errors.Trace(err)
	}

	gzr, err := gzip.NewReader(file)
	if err != nil {
		file.Close()
		return nil, errors.Trace(err)
	}

	return gzr, nil
}

// Metadata returns the metadata stored in the backup archive.  If no
// metadata is there, errors.NotFound is returned.
func (ad *ArchiveData) Metadata() (*metadata.Metadata, error) {
	file, err := ad.Open()
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer file.Close()

	filename := ad.MetadataFile()
	_, metaFile, err := tar.FindFile(file, filename)
	if err != nil {
		return nil, errors.Trace(err)
	}

	meta, err := metadata.NewFromJSONBuffer(metaFile)
	return meta, errors.Trace(err)
}

// Version returns the juju version under which the backup archive
// was created.  If no version is found in the archive, it must come
// from before backup archives included the version.  In that case we
// return version 1.20.
func (ad *ArchiveData) Version() (*version.Number, error) {
	meta, err := ad.Metadata()
	if errors.IsNotFound(err) {
		return &legacyVersion, nil
	}
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &meta.Origin.Version, nil
}
