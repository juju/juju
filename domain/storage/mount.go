// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"path"
	"strings"

	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/internal/errors"
)

// FilesystemMountPoint returns a mount point to use for the given charm
// storage. For stores with potentially multiple instances, the
// instance ID is appended to the location.
func FilesystemMountPoint(
	parentDir string,
	location string,
	maxCount int,
	storageID corestorage.ID,
) (string, error) {
	if parentDir == "" {
		return "", errors.New("empty parent directory not valid")
	}
	if strings.HasPrefix(location, parentDir) {
		return "", errors.Errorf(
			"invalid location %q: must not fall within %q",
			location, parentDir,
		)
	}
	if location != "" && maxCount == 1 {
		// The location is specified and it's a singleton
		// store, so just use the location as-is.
		return location, nil
	}
	// If the location is unspecified then we use
	// <parentDir>/<storage-id> as the location.
	// Otherwise, we use <location>/<storage-id>.
	if location != "" {
		parentDir = location
	}
	return path.Join(parentDir, storageID.String()), nil
}
