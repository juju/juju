// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"

	"github.com/juju/errors"

	"github.com/juju/juju/state/backups/metadata"
)

// File represents a file during testing.
type File struct {
	// Name is the path to which the file will be identified in the archive.
	Name string
	// Content is the data that will be written to the archive for the file.
	Content string
}

// Add the file to the tar archive.
func (f *File) AddToArchive(archive *tar.Writer) error {
	hdr := &tar.Header{
		Name: f.Name,
		Size: int64(len(f.Content)),
	}
	if err := archive.WriteHeader(hdr); err != nil {
		return errors.Trace(err)
	}
	if _, err := archive.Write([]byte(f.Content)); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// NewArchive returns a new archive file containing the files.
func NewArchive(meta *metadata.Metadata, files, dump []File) (*bytes.Buffer, error) {
	var rootFile bytes.Buffer
	if err := writeToTar(&rootFile, files); err != nil {
		return nil, errors.Trace(err)
	}

	metaFile, err := meta.AsJSONBuffer()
	if err != nil {
		return nil, errors.Trace(err)
	}

	topfiles := append(dump,
		File{
			Name:    "juju-backup/root.tar",
			Content: rootFile.String(),
		},
		File{
			Name:    "juju-backup/metadata.json",
			Content: metaFile.(*bytes.Buffer).String(),
		},
	)

	var arFile bytes.Buffer
	compressed := gzip.NewWriter(&arFile)
	defer compressed.Close()
	if err := writeToTar(compressed, topfiles); err != nil {
		return nil, errors.Trace(err)
	}

	return &arFile, nil
}

// NewArchiveBasic returns a new archive file with a few files provided.
func NewArchiveBasic(meta *metadata.Metadata) (*bytes.Buffer, error) {
	files := []File{
		File{
			Name:    "var/lib/juju/tools/1.21-alpha2.1-trusty-amd64/jujud",
			Content: "<some binary data goes here>",
		},
		File{
			Name:    "var/lib/juju/system-identity",
			Content: "<an ssh key goes here>",
		},
	}
	dump := []File{
		File{
			Name:    "juju-backup/dump/juju/machines.bson",
			Content: "<BSON data goes here>",
		},
		File{
			Name:    "juju-backup/dump/oplog.bson",
			Content: "<BSON data goes here>",
		},
	}

	arFile, err := NewArchive(meta, files, dump)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return arFile, nil
}

func writeToTar(archive io.Writer, files []File) error {
	tarw := tar.NewWriter(archive)
	defer tarw.Close()

	for _, file := range files {
		if err := file.AddToArchive(tarw); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}
