// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"path/filepath"

	"github.com/juju/errors"
)

// TODO(ericsnow) Pull these from authoritative sources:
var (
	dataDir        = "/var/lib/juju"
	startupDir     = "/etc/init"
	loggingConfDir = "/etc/rsyslog.d"
	logsDir        = "/var/log/juju"
	sshDir         = "/home/ubuntu/.ssh"

	machinesConfs = "jujud-machine-*.conf"
	agentsDir     = "agents"
	agentsConfs   = "machine-*"
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

// TODO(ericsnow) One concern is files that get out of date by the time
// backup finishes running.  This is particularly a problem with log
// files.

var getFilesToBackup = func(rootDir string) ([]string, error) {
	var glob string

	glob = filepath.Join(rootDir, startupDir, machinesConfs)
	initMachineConfs, err := filepath.Glob(glob)
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch machine init files")
	}

	glob = filepath.Join(rootDir, dataDir, agentsDir, agentsConfs)
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
		filepath.Join(rootDir, dataDir, toolsDir),

		filepath.Join(rootDir, dataDir, sshIdentFile),
		filepath.Join(rootDir, dataDir, nonceFile),
		filepath.Join(rootDir, logsDir, allMachinesLog),
		filepath.Join(rootDir, logsDir, machine0Log),
		filepath.Join(rootDir, sshDir, authKeysFile),

		filepath.Join(rootDir, startupDir, dbStartupConf),
		filepath.Join(rootDir, dataDir, dbPEM),
		filepath.Join(rootDir, dataDir, dbSecret),
	}
	backupFiles = append(backupFiles, initMachineConfs...)
	backupFiles = append(backupFiles, agentConfs...)
	backupFiles = append(backupFiles, jujuLogConfs...)
	return backupFiles, nil
}
