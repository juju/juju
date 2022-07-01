// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/juju/errors"

	"github.com/juju/juju/v2/mongo"
)

// TODO(ericsnow) lp-1392876
// Pull these from authoritative sources (see
// github.com/juju/juju/juju/paths, etc.):
const (
	dataDir = "/var/lib/juju"
	sshDir  = "/home/ubuntu/.ssh"

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

// Paths holds the paths that backups needs.
type Paths struct {
	BackupDir string
	DataDir   string
	LogsDir   string
}

// DiskUsage instances are used to find disk usage for a path.
type DiskUsage interface {
	Available(path string) uint64
}

// GetFilesToBackUp returns the paths that should be included in the
// backup archive.
func GetFilesToBackUp(rootDir string, paths *Paths) ([]string, error) {
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

// replaceableFolders for testing purposes.
var replaceableFolders = replaceableFoldersFunc

// replaceableFoldersFunc will return a map with the folders that need to
// be replaced so they can be deleted prior to a restore.
// Mongo 2.4 requires that the database directory be removed, while
// Mongo 3.2 requires that it not be removed
func replaceableFoldersFunc(dataDir string, mongoVersion mongo.Version) (map[string]os.FileMode, error) {
	replaceables := map[string]os.FileMode{}

	// NOTE: never put dataDir in here directly as that will unconditionally
	// remove the database.
	dirs := []string{
		filepath.Join(dataDir, "init"),
		filepath.Join(dataDir, "tools"),
		filepath.Join(dataDir, "agents"),
	}
	if mongoVersion.Major == 2 {
		dirs = append(dirs, filepath.Join(dataDir, "db"))
	}

	for _, replaceable := range dirs {
		dirStat, err := os.Stat(replaceable)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return map[string]os.FileMode{}, errors.Annotatef(err, "cannot stat %q", replaceable)
		}
		replaceables[replaceable] = dirStat.Mode()
	}
	return replaceables, nil
}

// TODO (perrito666) make this version sensitive when these files change, it would
// also be a good idea to save these instead of deleting them.

// PrepareMachineForRestore deletes all files from the re-bootstrapped
// machine that are to be replaced by the backup and recreates those
// directories that are to contain new files; this is to avoid
// possible mixup from new/old files that lead to an inconsistent
// restored state machine.
func PrepareMachineForRestore(mongoVersion mongo.Version) error {
	replaceFolders, err := replaceableFolders(dataDir, mongoVersion)
	if err != nil {
		return errors.Annotate(err, "cannot retrieve the list of folders to be cleaned before restore")
	}
	var keys []string
	for k := range replaceFolders {
		keys = append(keys, k)
	}
	// sort to avoid trying to create subfolders before folders.
	sort.Strings(keys)
	for _, toBeRecreated := range keys {
		fmode := replaceFolders[toBeRecreated]
		_, err := os.Stat(toBeRecreated)
		if err != nil && !os.IsNotExist(err) {
			return errors.Trace(err)
		}
		if !fmode.IsDir() {
			continue
		}
		logger.Debugf("removing dir: %s", toBeRecreated)
		if err := os.RemoveAll(toBeRecreated); err != nil {
			return errors.Trace(err)
		}
		if err := os.MkdirAll(toBeRecreated, fmode); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}
