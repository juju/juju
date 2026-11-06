// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

import (
	"io/fs"
	"math"
	"os"
	"path/filepath"

	"github.com/juju/juju/internal/errors"
)

// dqliteDataDir is the controller data directory holding the Dqlite files.
// It mirrors the layout defined by internal/database.
const dqliteDataDir = "dqlite"

// databaseSize records a capacity floor from the controller's Dqlite
// directory. It is a sizing observation, not a filesystem snapshot; recovery
// imports the logical dumps. Removed log segments need not contribute to this
// observation.
func databaseSize(dataDir string) (int64, error) {
	var total int64
	err := filepath.WalkDir(filepath.Join(dataDir, dqliteDataDir), func(path string, entry fs.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Size() < 0 || total > math.MaxInt64-info.Size() {
			return errors.New("source database size exceeds supported range")
		}
		total += info.Size()
		return nil
	})
	return total, err
}
