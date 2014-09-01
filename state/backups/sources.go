// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/utils/tar"

	"github.com/juju/juju/mongo"
	coreutils "github.com/juju/juju/utils"
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

var runCommand = coreutils.RunCommand

//---------------------------
// state-related files

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

func dumpFiles(dumpdir string) error {
	tarFile, err := os.Create(filepath.Join(dumpdir, "root.tar"))
	if err != nil {
		return errors.Annotate(err, "error while opening initial archive")
	}
	defer tarFile.Close()

	backupFiles, err := getFilesToBackup("")
	if err != nil {
		return errors.Annotate(err, "cannot determine files to backup")
	}

	sep := string(os.PathSeparator)
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
func (ci *dbConnInfo) UpdateFromMongoInfo(mgoInfo *mongo.MongoInfo) {
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
