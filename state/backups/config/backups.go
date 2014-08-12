// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

import (
	"os"
	"path/filepath"

	"github.com/juju/errors"

	"github.com/juju/juju/mongo"
)

// TODO(ericsnow) Pull these from elsewhere in juju.
var (
	// tools
	toolsDataFiles = []string{
		"tools",
	}

	// machines
	machinesDataFiles = []string{
		filepath.Join("agents", "machine-*"),
		"system-identity",
		"nonce.txt",
	}
	machinesStartupFiles = []string{
		"jujud-machine-*.conf",
	}
	machinesLoggingConfs = []string{
		"*juju.conf",
	}
	machinesLogFiles = []string{
		"all-machines.log",
		"machine-0.log",
	}
	machinesSSHFiles = []string{
		"authorized_keys",
	}

	// DB
	defaultDBDumpName = "mongodump"
	dbDataFiles       = []string{
		"server.pem",
		"shared-secret",
	}
	dbStartupFiles = []string{
		"juju-db.conf",
	}
)

// BackupsConfig is an abstraction of the info needed for backups.
type BackupsConfig interface {
	// FilesToBackUp returns the list of paths to files that should be
	// backed up.
	FilesToBackUp() ([]string, error)

	// DBDump returns the necessary information to call the dump command.
	DBDump(outDir string) (bin string, args []string, err error)
}

type backupsConfig struct {
	dbConnInfo DBConnInfo
	dbBinDir   string
	dbDumpName string

	paths Paths
}

// NewBackupsConfig returns a new backups config.
func NewBackupsConfig(
	addr, user, pw, dbBinDir string, paths Paths,
) (BackupsConfig, error) {
	if dbBinDir == "" {
		mongod, err := mongo.Path()
		if err != nil {
			return nil, errors.Annotate(err, "failed to get mongod path")
		}
		dbBinDir = filepath.Dir(mongod)
	}
	if paths == nil {
		var err error
		paths, err = NewPaths("", "", "", "", "", "")
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	config := backupsConfig{
		dbConnInfo: NewDBConnInfo(addr, user, pw),
		dbBinDir:   dbBinDir,
		dbDumpName: defaultDBDumpName,

		paths: paths,
	}
	return &config, nil
}

func findFiles(dir string, nameGroups ...[]string) ([]string, error) {
	files := []string{}
	for _, names := range nameGroups {
		for _, name := range names {
			glob := filepath.Join(dir, name)
			found, err := filepath.Glob(glob)
			if err != nil {
				err = errors.Annotatef(err, "error finding files (%s)", glob)
				return nil, err
			}
			if found == nil {
				return nil, errors.Errorf("no files found for %q", glob)
			}
			files = append(files, found...)
		}
	}
	return files, nil
}

func (bc *backupsConfig) FilesToBackUp() ([]string, error) {
	files := []string{}

	// data files
	found, err := findFiles(
		bc.paths.DataDir(),
		machinesDataFiles,
		dbDataFiles,
		toolsDataFiles,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	files = append(files, found...)

	// startup files
	found, err = findFiles(
		bc.paths.StartupDir(),
		machinesStartupFiles,
		dbStartupFiles,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	files = append(files, found...)

	// logging conf files
	found, err = findFiles(
		bc.paths.LoggingConfDir(),
		machinesLoggingConfs,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	files = append(files, found...)

	// log files
	found, err = findFiles(
		bc.paths.LogsDir(),
		machinesLogFiles,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	files = append(files, found...)

	// ssh files
	found, err = findFiles(
		bc.paths.SSHDir(),
		machinesSSHFiles,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	files = append(files, found...)

	return files, nil
}

func (bc *backupsConfig) DBDump(outDir string) (string, []string, error) {
	bin := filepath.Join(bc.dbBinDir, bc.dbDumpName)
	if _, err := os.Stat(bin); err != nil {
		return "", nil, errors.Annotatef(err, "missing %q", bin)
	}

	addr, user, pw, err := bc.dbConnInfo.Check()
	if err != nil {
		return "", nil, errors.Trace(err)
	}
	args := []string{
		"--oplog",
		"--ssl",
		"--host", addr,
		"--username", user,
		"--password", pw,
		"--out", outDir,
	}

	return bin, args, nil
}
