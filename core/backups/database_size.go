// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"errors"
	"io/fs"
	"math"
	"os"
	"path/filepath"
)

// DatabaseSize records a capacity floor from the source's Dqlite directory.
// It is a sizing observation, not a filesystem snapshot; recovery imports the
// logical dumps. Removed log segments need not contribute to this observation.
func DatabaseSize(dataDir string) (int64, error) {
	var total int64
	err := filepath.WalkDir(filepath.Join(dataDir, "dqlite"), func(path string, entry fs.DirEntry, err error) error {
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
