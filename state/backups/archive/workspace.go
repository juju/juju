// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package archive

import (
	"compress/gzip"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/utils/tar"

	"github.com/juju/juju/state/backups/metadata"
)

// Workspace is a wrapper around backup archive info that has a concrete
// root directory and an archive unpacked in it.
type Workspace struct {
	Archive
}

func newWorkspace() (*Workspace, error) {
	dirName, err := ioutil.TempDir("", "juju-backups-")
	if err != nil {
		return nil, errors.Annotate(err, "while creating workspace dir")
	}

	// Populate the workspace info.
	ws := Workspace{
		Archive: Archive{
			UnpackedRootDir: dirName,
		},
	}
	return &ws, nil
}

// NewWorkspace returns a new workspace with the compressed archive
// file unpacked into the workspace dir.
func NewWorkspace(archiveFile io.Reader) (*Workspace, error) {
	ws, err := newWorkspace()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Unpack the archive.
	tarFile, err := gzip.NewReader(archiveFile)
	if err != nil {
		return ws, errors.Annotate(err, "while uncompressing archive file")
	}
	if err := tar.UntarFiles(tarFile, dirName); err != nil {
		return ws, errors.Annotate(err, "while extracting files from archive")
	}
	return ws, nil
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

	if err := tar.UntarFiles(tarFile, targetRoot); err != nil {
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

	tarFile, err := os.Open(ws.FilesBundle())
	if err != nil {
		return nil, errors.Trace(err)
	}

	_, file, err := tar.FindFile(tarFile, filename)
	if err != nil {
		tarFile.Close()
		return nil, errors.Trace(err)
	}
	return file, nil
}

// Metadata returns the metadata derived from the JSON file in the archive.
func (ws *Workspace) Metadata() (*metadata.Metadata, error) {
	metaFile, err := os.Open(ws.MetadataFile())
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer metaFile.Close()

	meta, err := metadata.NewFromJSONBuffer(metaFile)
	return meta, errors.Trace(err)
}
