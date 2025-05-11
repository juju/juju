// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"path"
	"strings"

	"github.com/juju/juju/core/paths"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/internal/errors"
)

// storageParentDir is the parent directory for mounting charm storage.
var storageParentDir = paths.StorageDir(paths.OSUnixLike)

// FilesystemMountPoint returns a mount point to use for the given charm
// storage. For stores with potentially multiple instances, the
// instance ID is appended to the location.
func FilesystemMountPoint(
	location string,
	maxCount int,
	storageID corestorage.ID,
) (string, error) {
	parentDir := storageParentDir
	if strings.HasPrefix(location, path.Join(storageParentDir, "/")) {
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
