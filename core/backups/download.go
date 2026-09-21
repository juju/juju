// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"os"
	"path/filepath"
	"time"

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

// CleanExpiredOneShotArchives removes one-shot archives in the backup
// directory's one-shot download directory whose files are older than
// ttl, relative to now. A removal failure does not abort the sweep: the
// remaining archives are still processed and the failures are joined in
// the returned error so the caller can log them and retry next tick.
func CleanExpiredOneShotArchives(backupDir string, ttl time.Duration, now time.Time) error {
	dir := OneShotDir(backupDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return errors.Errorf("reading one-shot backup dir %q: %w", dir, err)
	}

	var errs []error
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			// Only regular files are staged by the API server;
			// anything else is not ours to remove.
			continue
		}
		info, err := entry.Info()
		if err != nil {
			errs = append(errs, errors.Errorf("reading one-shot backup file %q: %w", entry.Name(), err))
			continue
		}
		if now.Sub(info.ModTime()) <= ttl {
			continue
		}
		if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil && !os.IsNotExist(err) {
			errs = append(errs, errors.Errorf("removing expired one-shot backup %q: %w", entry.Name(), err))
		}
	}
	return errors.Join(errs...)
}
