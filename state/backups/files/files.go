// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package files

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/juju/errors"
)

// TODO(ericsnow) Pull these from authoritative sources (see
// github.com/juju/juju/juju/paths, etc.):
const (
	dataDir        = "/var/lib/juju"
	startupDir     = "/etc/init"
	loggingConfDir = "/etc/rsyslog.d"
	logsDir        = "/var/log/juju"
	sshDir         = "/home/ubuntu/.ssh"

	machinesConfs = "jujud-machine-*.conf"
	agentsDir     = "agents"
	agentsConfs   = "machine-*"
	jujuInitConfs = "juju-*.conf"
	loggingConfs  = "*juju.conf"
	toolsDir      = "tools"

	sshIdentFile   = "system-identity"
	nonceFile      = "nonce.txt"
	allMachinesLog = "all-machines.log"
	machine0Log    = "machine-0.log"
	authKeysFile   = "authorized_keys"

	dbStartupConf = "juju-db.conf"
	dbPEM         = "server.pem"
	dbSecret      = "shared-secret"
)

// Paths holds the paths that backups needs.
type Paths struct {
	DataDir string
	LogsDir string
}

// GetFilesToBackUp returns the paths that should be included in the
// backup archive.
func GetFilesToBackUp(rootDir string, paths Paths) ([]string, error) {
	var glob string

	glob = filepath.Join(rootDir, startupDir, machinesConfs)
	initMachineConfs, err := filepath.Glob(glob)
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch machine init files")
	}

	glob = filepath.Join(rootDir, startupDir, jujuInitConfs)
	initConfs, err := filepath.Glob(glob)
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch startup conf files")
	}

	glob = filepath.Join(rootDir, paths.DataDir, agentsDir, agentsConfs)
	agentConfs, err := filepath.Glob(glob)
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch agent config files")
	}

	glob = filepath.Join(rootDir, loggingConfDir, loggingConfs)
	jujuLogConfs, err := filepath.Glob(glob)
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch juju log conf files")
	}

	backupFiles := []string{
		filepath.Join(rootDir, paths.DataDir, toolsDir),

		filepath.Join(rootDir, paths.DataDir, sshIdentFile),
		filepath.Join(rootDir, paths.LogsDir, allMachinesLog),
		filepath.Join(rootDir, paths.LogsDir, machine0Log),

		filepath.Join(rootDir, paths.DataDir, dbPEM),
		filepath.Join(rootDir, paths.DataDir, dbSecret),
	}
	backupFiles = append(backupFiles, initMachineConfs...)
	backupFiles = append(backupFiles, agentConfs...)
	backupFiles = append(backupFiles, initConfs...)
	backupFiles = append(backupFiles, jujuLogConfs...)

	// Handle nonce.txt (might not exist).
	nonce := filepath.Join(rootDir, paths.DataDir, nonceFile)
	if _, err := os.Stat(nonce); err != nil {
		if !os.IsNotExist(err) {
			return nil, errors.Trace(err)
		}
	} else {
		backupFiles = append(backupFiles, nonce)
	}

	// Handle user SSH files (might not exist).
	SSHDir := filepath.Join(rootDir, sshDir)
	if _, err := os.Stat(SSHDir); err != nil {
		if !os.IsNotExist(err) {
			return nil, errors.Trace(err)
		}
	} else {
		backupFiles = append(backupFiles, filepath.Join(SSHDir, authKeysFile))
	}

	return backupFiles, nil
}

// replaceableFolders for testing purposes
var replaceableFolders = replaceableFoldersFunc

// replaceableFoldersFunc will return a map with the files/folders that need to
// be replaces so they can be deleted prior to a restore.
func replaceableFoldersFunc() (map[string]os.FileMode, error) {
	replaceables := map[string]os.FileMode{}

	for _, replaceable := range []string{
		filepath.Join(dataDir, "db"),
		dataDir,
		logsDir,
	} {
		dirStat, err := os.Stat(replaceable)
		if err != nil {
			return map[string]os.FileMode{}, errors.Annotatef(err, "cannot stat %q", replaceable)
		}
		replaceables[replaceable] = dirStat.Mode()
	}
	return replaceables, nil
}

// PrepareMachineForRestore deletes all files from the re-bootstrapped
// machine that are to be replaced by the backup and recreates those
// directories that are to contain new files; this is to avoid
// possible mixup from new/old files that lead to an inconsistent
// restored state machine.
func PrepareMachineForRestore() error {
	replaceFolders, err := replaceableFolders()
	if err != nil {
		return errors.Annotate(err, "cannot retrieve the list of folders to be cleaned before restore")
	}
	var keys []string
	for k := range replaceFolders {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, toBeRecreated := range keys {
		fmode := replaceFolders[toBeRecreated]
		_, err := os.Stat(toBeRecreated)
		if err != nil && !os.IsNotExist(err) {
			return errors.Trace(err)
		}
		if err := os.RemoveAll(toBeRecreated); err != nil {
			return errors.Trace(err)
		}
		if err := os.MkdirAll(toBeRecreated, fmode); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}
