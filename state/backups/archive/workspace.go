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

	ws := Workspace{
		Archive: Archive{"", dirName},
	}
	return &ws, nil
}

// NewWorkspaceFromFilename creates a new workspace for backup archives.
// The workspace is based in a new directory that will be deleted when
// the workspace is closed.  The given filename is opened and unpacked
// into the workspace.
func NewWorkspaceFromFilename(filename string) (*Workspace, error) {
	if filename == "" {
		return nil, errors.Errorf("missing filename")
	}

	ws, err := newWorkspace()
	if err != nil {
		return nil, errors.Trace(err)
	}
	ws.Filename = filename

	archiveFile, err := os.Open(filename)
	if err != nil {
		return nil, errors.Trace(err)
	}

	err = unpack(archiveFile, ws.UnpackedRootDir)
	return ws, errors.Trace(err)
}

// NewWorkspace returns a new archive workspace with a new workspace dir
// populated from the archive file.
func NewWorkspaceFromFile(archiveFile io.Reader) (*Workspace, error) {
	ws, err := newWorkspace()
	if err != nil {
		return nil, errors.Trace(err)
	}
	err = unpack(archiveFile, ws.UnpackedRootDir)
	return ws, errors.Trace(err)
}

func unpack(tarFile io.Reader, targetDir string) error {
	tarFile, err := gzip.NewReader(tarFile)
	if err != nil {
		return errors.Annotate(err, "while uncompressing archive file")
	}
	if err := tar.UntarFiles(tarFile, targetDir); err != nil {
		return errors.Annotate(err, "while extracting files from archive")
	}
	return nil
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
func (ws *Workspace) OpenFile(filename string) (io.Reader, error) {
	if filepath.IsAbs(filename) {
		return nil, errors.Errorf("filename must be relative, got %q", filename)
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
