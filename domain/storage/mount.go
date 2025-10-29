// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"path"
	"strconv"
	"strings"

	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/internal/errors"
)

// storageParentDir is the parent directory for mounting charm storage.
var storageParentDir = paths.StorageDir(paths.OSUnixLike)

// FilesystemMountPointK8s returns a mount point to use for a given filesystem
// to be mounted in homogenous k8s units.
func FilesystemMountPointK8s(
	location string,
	maxCount int, idx int,
	storageName string,
) (string, error) {
	if strings.HasPrefix(location, path.Join(storageParentDir, "/")) {
		return "", errors.Errorf(
			"invalid location %q: must not fall within %q",
			location, storageParentDir,
		)
	}
	if location != "" && maxCount == 1 {
		// The location is specified and it's a singleton
		// store, so just use the location as-is.
		return location, nil
	}
	parentDir := storageParentDir
	// If the location is unspecified then we use
	// <parentDir>/<storage-id> as the location.
	// Otherwise, we use <location>/<storage-id>.
	if location != "" {
		parentDir = location
	}
	return path.Join(parentDir, storageName, strconv.Itoa(idx)), nil
}
