// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/juju/errors"

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

//---------------------------
// database

const dumpName = "mongodump"

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

type mongoDumper struct {
	address  string
	username string
	password string
}

// NewDBDumper returns a new value with a Dump method for dumping the
// juju state database.
func NewDBDumper(mgoInfo *authentication.MongoInfo) *mongoDumper {
	dumper := mongoDumper{
		address:  mgoInfo.Addrs[0],
		password: mgoInfo.Password,
	}

	// TODO(dfc) Backup should take a Tag.
	if mgoInfo.Tag != nil {
		dumper.username = mgoInfo.Tag.String()
	}

	return &dumper
}

// Dump dumps the juju state database.
func (md *mongoDumper) Dump(dumpDir string) error {
	if md.address == "" {
		return errors.New("missing address")
	}
	if md.username == "" {
		return errors.New("missing username")
	}
	if md.password == "" {
		return errors.New("missing password")
	}

	mongodumpPath, err := getMongodumpPath()
	if err != nil {
		return errors.Annotate(err, "mongodump not available")
	}

	err = runCommand(
		mongodumpPath,
		"--oplog",
		"--ssl",
		"--host", md.address,
		"--username", md.username,
		"--password", md.password,
		"--out", dumpDir,
	)
	if err != nil {
		return errors.Annotate(err, "error dumping database")
	}

	return nil
}
