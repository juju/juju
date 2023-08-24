// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"os"
	"path/filepath"

	"github.com/juju/errors"

	corebackups "github.com/juju/juju/core/backups"
	"github.com/juju/juju/internal/mongo"
)

// TODO(ericsnow) lp-1392876
// Pull these from authoritative sources (see
// github.com/juju/juju/juju/paths, etc.):
const (
	sshDir = "/home/ubuntu/.ssh"

	agentsDir   = "agents"
	agentsConfs = "machine-*"
	toolsDir    = "tools"
	initDir     = "init"

	sshIdentFile = "system-identity"
	nonceFile    = "nonce.txt"
	authKeysFile = "authorized_keys"

	dbPEM           = mongo.FileNameDBSSLKey
	dbSecret        = "shared-secret"
	dbSecretSnapDir = "/var/snap/juju-db/common"
)

// BackupDirToUse returns the desired backup staging dir.
func BackupDirToUse(configuredDir string) string {
	if configuredDir != "" {
		return configuredDir
	}
	return os.TempDir()
}

// DiskUsage instances are used to find disk usage for a path.
type DiskUsage interface {
	Available(path string) uint64
}

// GetFilesToBackUp returns the paths that should be included in the
// backup archive.
func GetFilesToBackUp(rootDir string, paths *corebackups.Paths) ([]string, error) {
	var glob string

	glob = filepath.Join(rootDir, paths.DataDir, agentsDir, agentsConfs)
	agentConfs, err := filepath.Glob(glob)
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch agent config files")
	}

	glob = filepath.Join(rootDir, paths.DataDir, initDir, "*")
	serviceConfs, err := filepath.Glob(glob)
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch service config files")
	}

	backupFiles := []string{
		filepath.Join(rootDir, paths.DataDir, toolsDir),
		filepath.Join(rootDir, paths.DataDir, sshIdentFile),
		filepath.Join(rootDir, paths.DataDir, dbPEM),
	}

	// Handle shared-secret (may be in /var/lib/juju or /var/snap/juju-db/common).
	secret := filepath.Join(rootDir, paths.DataDir, dbSecret)
	if _, err := os.Stat(secret); err != nil {
		if !os.IsNotExist(err) {
			return nil, errors.Trace(err)
		}
		secretSnap := filepath.Join(rootDir, dbSecretSnapDir, dbSecret)
		logger.Tracef("shared-secret not found at %q, trying %q", secret, secretSnap)
		if _, err := os.Stat(secretSnap); err != nil {
			return nil, errors.Trace(err)
		}
		secret = secretSnap
	}
	backupFiles = append(backupFiles, secret)

	backupFiles = append(backupFiles, agentConfs...)
	backupFiles = append(backupFiles, serviceConfs...)

	// Handle nonce.txt (might not exist).
	nonce := filepath.Join(rootDir, paths.DataDir, nonceFile)
	if _, err := os.Stat(nonce); err != nil {
		if !os.IsNotExist(err) {
			return nil, errors.Trace(err)
		}
		logger.Errorf("skipping missing file %q", nonce)
	} else {
		backupFiles = append(backupFiles, nonce)
	}

	// Handle user SSH files (might not exist).
	SSHDir := filepath.Join(rootDir, sshDir)
	if _, err := os.Stat(SSHDir); err != nil {
		if !os.IsNotExist(err) {
			return nil, errors.Trace(err)
		}
		logger.Errorf("skipping missing dir %q", SSHDir)
	} else {
		backupFiles = append(backupFiles, filepath.Join(SSHDir, authKeysFile))
	}

	return backupFiles, nil
}
