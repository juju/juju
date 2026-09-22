// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"path/filepath"

	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

const (
	// OneShotDirName is the name of the directory, relative to the
	// backup directory, in which archives pending one-shot download are
	// staged under server-minted ids.
	OneShotDirName = "downloads"

	// oneShotArchiveSuffix is the suffix of one-shot archive filenames.
	// The base name is the backup ID: a UUID minted by the API server.
	oneShotArchiveSuffix = ".tar.gz"
)

// OneShotDir returns the path of the one-shot download directory under
// the given backup directory.
func OneShotDir(backupDir string) string {
	return filepath.Join(backupDir, OneShotDirName)
}

// OneShotArchivePath returns the path of the one-shot archive for the
// given backup ID within the backup directory. The ID is validated as a
// UUID: IDs are minted by the API server and must never be treated as
// filesystem input.
func OneShotArchivePath(backupDir, id string) (string, error) {
	if !uuid.IsValidUUIDString(id) {
		return "", errors.Errorf("invalid backup id %q", id)
	}
	return filepath.Join(OneShotDir(backupDir), id+oneShotArchiveSuffix), nil
}
