// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package files

import (
	"os"
	"path/filepath"

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
		filepath.Join(rootDir, paths.DataDir, nonceFile),
		filepath.Join(rootDir, paths.LogsDir, allMachinesLog),
		filepath.Join(rootDir, paths.LogsDir, machine0Log),

		filepath.Join(rootDir, paths.DataDir, dbPEM),
		filepath.Join(rootDir, paths.DataDir, dbSecret),
	}
	backupFiles = append(backupFiles, initMachineConfs...)
	backupFiles = append(backupFiles, agentConfs...)
	backupFiles = append(backupFiles, initConfs...)
	backupFiles = append(backupFiles, jujuLogConfs...)

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
