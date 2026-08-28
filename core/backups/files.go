// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/juju/internal/errors"
)

// TODO(ericsnow) lp-1392876
// Pull these from authoritative sources (see
// github.com/juju/juju/juju/paths, etc.):
const (
	sshDir = "/home/ubuntu/.ssh"

	agentsDir      = "agents"
	agentsConfs    = "machine-*"
	toolsDir       = "tools"
	initDir        = "init"
	objectstoreDir = "objectstore"

	sshIdentFile = "system-identity"
	serverPEM    = "server.pem"
	dbSecret     = "shared-secret"
	nonceFile    = "nonce.txt"
	authKeysFile = "authorized_keys"
)

// BackupDirToUse returns the desired backup staging dir.
func BackupDirToUse(configuredDir string) string {
	if configuredDir != "" {
		return configuredDir
	}
	return os.TempDir()
}

// GetFilesToBackUp returns the paths that should be included in the
// backup archive.
func GetFilesToBackUp(rootDir string, paths *Paths) ([]string, error) {
	glob := filepath.Join(rootDir, paths.DataDir, agentsDir, agentsConfs)
	agentConfs, err := filepath.Glob(glob)
	if err != nil {
		return nil, errors.Errorf("failed to fetch agent config files: %w", err)
	}

	glob = filepath.Join(rootDir, paths.DataDir, initDir, "*")
	serviceConfs, err := filepath.Glob(glob)
	if err != nil {
		return nil, errors.Errorf("failed to fetch service config files: %w", err)
	}

	// The objectstore (charms, resources and agent binaries) and the
	// tools directory must be there; a failure to walk them is fatal.
	backupFiles := []string{
		filepath.Join(rootDir, paths.DataDir, objectstoreDir),
		filepath.Join(rootDir, paths.DataDir, toolsDir),
	}

	backupFiles = append(backupFiles, agentConfs...)
	backupFiles = append(backupFiles, serviceConfs...)

	// The following files might not exist; skip the missing ones.
	optional := []string{
		filepath.Join(rootDir, paths.DataDir, sshIdentFile),
		filepath.Join(rootDir, paths.DataDir, serverPEM),
		filepath.Join(rootDir, paths.DataDir, dbSecret),
		filepath.Join(rootDir, paths.DataDir, nonceFile),
		filepath.Join(rootDir, sshDir, authKeysFile),
	}
	for _, file := range optional {
		if _, err := os.Stat(file); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return nil, errors.Capture(err)
			}
			continue
		}
		backupFiles = append(backupFiles, file)
	}

	var finalBackupFiles []string
	for _, file := range backupFiles {
		err := filepath.Walk(file,
			func(path string, info fs.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if info.IsDir() {
					return nil
				}
				if info.Mode().IsRegular() ||
					info.Mode()&os.ModeSymlink != 0 {
					finalBackupFiles = append(finalBackupFiles, path)
				}
				return nil
			})
		if err != nil {
			return nil, errors.Errorf("cannot walk %q: %w", file, err)
		}
	}
	return finalBackupFiles, nil
}

// IsValidBackupFilepath reports whether filePath names an existing regular
// file directly under root whose base name starts with [FilenamePrefix]. It
// is used by the download handler to reject arbitrary paths while allowing
// absolute client-provided ids.
func IsValidBackupFilepath(root string, filePath string) (bool, error) {
	if !filepath.IsAbs(filePath) {
		return false, nil
	}
	if !strings.HasPrefix(filepath.Base(filePath), FilenamePrefix) {
		return false, nil
	}
	result := false
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Best-effort validation: unreadable entries are skipped.
			return nil
		}
		if !d.IsDir() && path == filePath {
			result = true
		}
		return nil
	})
	return result, walkErr
}
