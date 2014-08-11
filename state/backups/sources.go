// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/utils/tar"

	"github.com/juju/juju/environmentserver/authentication"
	"github.com/juju/juju/mongo"
	coreutils "github.com/juju/juju/utils"
)

var sep = string(os.PathSeparator)

var runCommand = coreutils.RunCommand

//---------------------------
// state-related files

// TODO(ericsnow) One concern is files that get out of date by the time
// backup finishes running.  This is particularly a problem with log
// files.

var getFilesToBackup = func() ([]string, error) {
	const dataDir string = "/var/lib/juju"
	initMachineConfs, err := filepath.Glob("/etc/init/jujud-machine-*.conf")
	if err != nil {
		return nil, errors.Annotate(
			err, "failed to fetch machine upstart files")
	}
	agentConfs, err := filepath.Glob(filepath.Join(
		dataDir, "agents", "machine-*"))
	if err != nil {
		return nil, errors.Annotate(
			err, "failed to fetch agent configuration files")
	}
	jujuLogConfs, err := filepath.Glob("/etc/rsyslog.d/*juju.conf")
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch juju log conf files")
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

func dumpFiles(dumpdir string) error {
	tarFile, err := os.Create(filepath.Join(dumpdir, "root.tar"))
	if err != nil {
		return errors.Annotate(err, "error while opening initial archive")
	}
	defer tarFile.Close()
	backupFiles, err := getFilesToBackup()
	if err != nil {
		return errors.Annotate(err, "cannot determine files to backup")
	}
	_, err = tar.TarFiles(backupFiles, tarFile, sep)
	if err != nil {
		return errors.Annotate(err, "cannot backup configuration files")
	}
	return nil
}

//---------------------------
// database

const dumpName = "mongodump"

// DBConnInfo is a simplification of authentication.MongoInfo.
type DBConnInfo interface {
	Address() string
	Username() string
	Password() string
}

type dbConnInfo struct {
	address  string
	username string
	password string
}

// NewDBConnInfo returns a new DBConnInfo.
func NewDBConnInfo(addr, user, pw string) DBConnInfo {
	dbinfo := dbConnInfo{
		address:  addr,
		username: user,
		password: pw,
	}
	return &dbinfo
}

// Address returns the connection address.
func (ci *dbConnInfo) Address() string {
	return ci.address
}

// Username returns the connection username.
func (ci *dbConnInfo) Username() string {
	return ci.username
}

// Password returns the connection password.
func (ci *dbConnInfo) Password() string {
	return ci.password
}

// UpdateFromMongoInfo pulls in the provided connection info.
func (ci *dbConnInfo) UpdateFromMongoInfo(mgoInfo *authentication.MongoInfo) {
	ci.address = mgoInfo.Addrs[0]
	ci.password = mgoInfo.Password

	// TODO(dfc) Backup should take a Tag.
	if mgoInfo.Tag != nil {
		ci.username = mgoInfo.Tag.String()
	}
}

var getMongodumpPath = func() (string, error) {
	mongod, err := mongo.Path()
	if err != nil {
		return "", errors.Annotate(err, "failed to get mongod path")
	}
	mongoDumpPath := filepath.Join(filepath.Dir(mongod), dumpName)

	if _, err := os.Stat(mongoDumpPath); err == nil {
		// It already exists so no need to continue.
		return mongoDumpPath, nil
	}

	path, err := exec.LookPath(dumpName)
	if err != nil {
		return "", errors.Trace(err)
	}
	return path, nil
}

func dumpDatabase(info DBConnInfo, dirname string) error {
	mongodumpPath, err := getMongodumpPath()
	if err != nil {
		return errors.Annotate(err, "mongodump not available")
	}

	err = runCommand(
		mongodumpPath,
		"--oplog",
		"--ssl",
		"--host", info.Address(),
		"--username", info.Username(),
		"--password", info.Password(),
		"--out", dirname,
	)
	if err != nil {
		return errors.Annotate(err, "failed to dump database")
	}

	return nil
}
