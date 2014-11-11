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

// NewWorkspaceReader returns a new archive workspace with a new
// workspace dir populated from the archive file.
func NewWorkspaceReader(archiveFile io.Reader) (*Workspace, error) {
	ws, err := newWorkspace()
	if err != nil {
		return nil, errors.Trace(err)
	}
	err = unpack(ws.UnpackedRootDir, archiveFile)
	return ws, errors.Trace(err)
}

func unpack(targetDir string, tarFile io.Reader) error {
	tarFile, err := gzip.NewReader(tarFile)
	if err != nil {
		return errors.Annotate(err, "while uncompressing archive file")
	}
	err = tar.UntarFiles(tarFile, targetDir)
	return errors.Trace(err)
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

	err = tar.UntarFiles(tarFile, targetRoot)
	return errors.Trace(err)
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

	meta, err := metadata.NewMetadataJSONReader(metaFile)
	return meta, errors.Trace(err)
}
