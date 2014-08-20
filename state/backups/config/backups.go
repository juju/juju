// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

import (
	"os"
	"path/filepath"

	"github.com/juju/errors"
)

var filesToBackUp = [][]string{
	// TODO(ericsnow) Pull these from elsewhere in juju.

	// tools
	{"data", "tools/"},

	// machines
	{"data", filepath.Join("agents", "machine-*")},
	{"data", "system-identity"},
	{"data", "nonce.txt"},
	{"startup", "jujud-machine-*.conf"},
	{"loggingConf", "*juju.conf"},
	{"logs", "all-machines.log"},
	{"logs", "machine-0.log"},
	{"ssh", "authorized_keys"},

	// DB
	{"data", "server.pem"},
	{"data", "shared-secret"},
	{"startup", "juju-db.conf"},
}

// BackupsConfig is an abstraction of the information needed for all
// backups-related functionality.  Its methods expose only the specific
// information needed for existing functionality.  However, as a whole
// BackupsConfig represents any type that encapsulates all the external
// information needed by backups-related functionality.  As the backups
// API grows this type becomes more valuable, particularly where it
// provides information that is common across the backups API.
type BackupsConfig interface {
	// FilesToBackUp returns the list of paths to files that should be
	// backed up, based on the config.
	FilesToBackUp() ([]string, error)
	// DBDump returns the necessary information to call the dump command.
	// This info is derived from the underlying config.
	DBDump(outDir string) (bin string, args []string, err error)
}

type backupsConfig struct {
	dbInfo DBInfo
	paths  Paths
}

// NewBackupsConfig returns a new backups config.
func NewBackupsConfig(dbInfo DBInfo, paths Paths) (BackupsConfig, error) {
	if dbInfo == nil {
		return nil, errors.New("missing dbInfo")
	}
	if paths == nil {
		paths = &DefaultPaths
	}

	config := backupsConfig{
		dbInfo: dbInfo,
		paths:  paths,
	}
	return &config, nil
}

func (bc *backupsConfig) FilesToBackUp() ([]string, error) {
	filenames, err := bc.paths.FindEvery(filesToBackUp...)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return filenames, nil
}

func (bc *backupsConfig) DBDump(outDir string) (string, []string, error) {
	bin := bc.dbInfo.DumpBinary()
	if _, err := os.Stat(bin); err != nil {
		return "", nil, errors.Annotatef(err, "missing %q", bin)
	}

	addr, user, pw, err := bc.dbInfo.ConnInfo().Check()
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
