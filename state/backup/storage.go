// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

import (
	"path"
	"path/filepath"
)

// StorageName returns the path in environment storage where a backup
// should be stored.
func StorageName(filename string) string {
	// Use of path.Join instead of filepath.Join is intentional - this
	// is an environment storage path not a filesystem path.
	return path.Join("/backups", filepath.Base(filename))
}
