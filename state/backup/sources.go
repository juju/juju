// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/juju/utils"
)

//---------------------------
// state-related files (configs, logs, keys)

var getFilesToBackup = _getFilesToBackup

func _getFilesToBackup() ([]string, error) {
	const dataDir string = "/var/lib/juju"
	initMachineConfs, err := filepath.Glob("/etc/init/jujud-machine-*.conf")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch machine upstart files: %v", err)
	}
	agentConfs, err := filepath.Glob(filepath.Join(dataDir, "agents", "machine-*"))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch agent configuration files: %v", err)
	}
	jujuLogConfs, err := filepath.Glob("/etc/rsyslog.d/*juju.conf")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch juju log conf files: %v", err)
	}

	backupFiles := []string{
		"/etc/init/juju-db.conf",
		filepath.Join(dataDir, "tools"),
		filepath.Join(dataDir, "server.pem"),
		filepath.Join(dataDir, "system-identity"),
		filepath.Join(dataDir, "nonce.txt"),
		filepath.Join(dataDir, "shared-secret"),
		"/home/ubuntu/.ssh/authorized_keys",
		"/var/log/juju/all-machines.log",
		"/var/log/juju/machine-0.log",
	}
	backupFiles = append(backupFiles, initMachineConfs...)
	backupFiles = append(backupFiles, agentConfs...)
	backupFiles = append(backupFiles, jujuLogConfs...)
	return backupFiles, nil
}

//---------------------------
// the database

var runCommand = _runCommand

func _runCommand(cmd string, args ...string) error {
	out, err := utils.RunCommand(cmd, args...)
	if err == nil {
		return nil
	}
	if _, ok := err.(*exec.ExitError); ok && len(out) > 0 {
		msg := strings.Replace(string(out), "\n", "; ", -1)
		return fmt.Errorf("error executing %q: %s", cmd, msg)
	} else {
		return fmt.Errorf("cannot execute %q: %v", cmd, err)
	}
}

var getMongodumpPath = _getMongodumpPath

func _getMongodumpPath() (string, error) {
	const mongoDumpPath string = "/usr/lib/juju/bin/mongodump"

	if _, err := os.Stat(mongoDumpPath); err == nil {
		return mongoDumpPath, nil
	}

	path, err := exec.LookPath("mongodump")
	if err != nil {
		return "", err
	}
	return path, nil
}

func dumpDatabase(dbinfo *DBConnInfo, dumpDir string) error {
	mongodumpPath, err := getMongodumpPath()
	if err != nil {
		return fmt.Errorf("mongodump not available: %v", err)
	}
	err = runCommand(
		mongodumpPath,
		"--oplog",
		"--ssl",
		"--host", dbinfo.Hostname,
		"--username", dbinfo.Username,
		"--password", dbinfo.Password,
		"--out", dumpDir,
	)
	if err != nil {
		return fmt.Errorf("failed to dump database: %v", err)
	}
	return nil
}
