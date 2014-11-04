// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package archive

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	utar "github.com/juju/utils/tar"
)

// Workspace is a wrapper around backup archive info that has a concrete
// root directory and an archive unpacked in it.
type Workspace struct {
	Archive
}

// NewWorkspace returns a new workspace with the compressed archive
// file unpacked into the workspace dir.
func NewWorkspace(archiveFile io.Reader) (*Workspace, error) {
	dirName, err := ioutil.TempDir("", "juju-backups-")
	if err != nil {
		return nil, errors.Annotate(err, "while creating workspace dir")
	}

	// Unpack the archive.
	tarFile, err := gzip.NewReader(archiveFile)
	if err != nil {
		return nil, errors.Annotate(err, "while uncompressing archive file")
	}
	if err := utar.UntarFiles(tarFile, dirName); err != nil {
		return nil, errors.Annotate(err, "while extracting files from archive")
	}

	// Populate the workspace info.
	ws := Workspace{
		Archive: Archive{
			UnpackedRootDir: dirName,
		},
	}
	return &ws, nil
}

// Close cleans up the workspace dir.
func (ws *Workspace) Close() error {
	err := os.RemoveAll(ws.UnpackedRootDir)
	return errors.Trace(err)
}

// UnpackFiles unpacks the archived files bundle into the targeted dir.
func (ws *Workspace) UnpackFiles(targetRoot string) error {
	tarFile, err := os.Open(ws.FilesBundle())
	if err != nil {
		return errors.Trace(err)
	}
	defer tarFile.Close()

	if err := utar.UntarFiles(tarFile, targetRoot); err != nil {
		return errors.Annotate(err, "while unpacking system files")
	}

	return nil
}

// OpenFile returns an open ReadCloser for the corresponding file in
// the archived files bundle.
func (ws *Workspace) OpenFile(filename string) (_ io.ReadCloser, err error) {
	if filepath.IsAbs(filename) {
		return nil, errors.Errorf("filename must not be relative, got %q", filename)
	}

	// TODO(ericsnow) This should go in utils/tar.

	tarFile, err := os.Open(ws.FilesBundle())
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer func() {
		if err != nil {
			tarFile.Close()
		}
	}()

	reader := tar.NewReader(tarFile)
	for {
		header, err := reader.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, errors.Trace(err)
		}

		if header.Name == filename {
			return ioutil.NopCloser(reader), nil
		}
	}

	return nil, errors.NotFoundf(filename)
}
