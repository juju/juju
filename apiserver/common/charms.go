// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"archive/zip"
	"io"
	"os"
	"path"
	"path/filepath"

	"github.com/juju/errors"

	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/rpc/params"
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

	charmFile, err := os.CreateTemp(tmpDir, "charm")
	if err != nil {
		return "", errors.Annotate(err, "cannot create charm archive file")
	}
	defer charmFile.Close()

	if _, err = io.Copy(charmFile, reader); err != nil {
		return "", errors.Annotate(err, "error processing charm archive download")
	}
	return charmFile.Name(), nil
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
		return io.ReadAll(contents)
	}
	if wantIcon {
		// An icon was requested but none was found in the archive so
		// return the default icon instead.
		return []byte(DefaultCharmIcon), nil
	}
	return nil, errors.NotFoundf("charm file")
}

// ValidateCharmOrigin validates the Source of the charm origin for args received
// in a facade. This may evolve over time to include more pieces.
func ValidateCharmOrigin(o *params.CharmOrigin) error {
	switch {
	case o == nil:
		return errors.BadRequestf("charm origin source required")
	case corecharm.CharmHub.Matches(o.Source):
		// If either the charm origin ID or Hash is set before a charm is
		// downloaded, charm download will fail for charms with a forced series.
		// The logic (refreshConfig) in sending the correct request to charmhub
		// will break.
		if (o.ID != "" && o.Hash == "") ||
			(o.ID == "" && o.Hash != "") {
			return errors.BadRequestf("programming error, both CharmOrigin ID and Hash must be set or neither. See CharmHubRepository GetDownloadURL.")
		}
	case corecharm.Local.Matches(o.Source):
	default:
		return errors.BadRequestf("%q not a valid charm origin source", o.Source)
	}
	return nil
}
