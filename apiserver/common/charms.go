// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"archive/zip"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/storage"
)

// ReadCharmFromStorage fetches the charm at the specified path from the store
// and copies it to a temp directory in dataDir.
func ReadCharmFromStorage(store storage.Storage, dataDir, storagePath string) (string, error) {
	// Ensure the working directory exists.
	tmpDir := filepath.Join(dataDir, "charm-get-tmp")
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return "", errors.Annotate(err, "cannot create charms tmp directory")
	}

	// Use the storage to retrieve and save the charm archive.
	reader, _, err := store.Get(storagePath)
	if err != nil {
		return "", errors.Annotate(err, "cannot get charm from model storage")
	}
	defer reader.Close()

	charmFile, err := ioutil.TempFile(tmpDir, "charm")
	if err != nil {
		return "", errors.Annotate(err, "cannot create charm archive file")
	}
	if _, err = io.Copy(charmFile, reader); err != nil {
		cleanupFile(charmFile)
		return "", errors.Annotate(err, "error processing charm archive download")
	}
	charmFile.Close()
	return charmFile.Name(), nil
}

// On windows we cannot remove a file until it has been closed
// If this poses an active problem somewhere else it will be refactored in
// utils and used everywhere.
func cleanupFile(file *os.File) {
	// Errors are ignored because it is ok for this to be called when
	// the file is already closed or has been moved.
	file.Close()
	os.Remove(file.Name())
}

// CharmArchiveEntry retrieves the specified entry from the zip archive.
func CharmArchiveEntry(charmPath, entryPath string, wantIcon bool) ([]byte, error) {
	// TODO(fwereade) 2014-01-27 bug #1285685
	// This doesn't handle symlinks helpfully, and should be talking in
	// terms of bundles rather than zip readers; but this demands thought
	// and design and is not amenable to a quick fix.
	zipReader, err := zip.OpenReader(charmPath)
	if err != nil {
		return nil, errors.Annotatef(err, "unable to read charm")
	}
	defer zipReader.Close()
	for _, file := range zipReader.File {
		if path.Clean(file.Name) != entryPath {
			continue
		}
		fileInfo := file.FileInfo()
		if fileInfo.IsDir() {
			return nil, &params.Error{
				Message: "directory listing not allowed",
				Code:    params.CodeForbidden,
			}
		}
		contents, err := file.Open()
		if err != nil {
			return nil, errors.Annotatef(err, "unable to read file %q", entryPath)
		}
		defer contents.Close()
		return ioutil.ReadAll(contents)
	}
	if wantIcon {
		// An icon was requested but none was found in the archive so
		// return the default icon instead.
		return []byte(DefaultCharmIcon), nil
	}
	return nil, errors.NotFoundf("charm file")
}
