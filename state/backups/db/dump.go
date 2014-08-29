// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package db

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/juju/errors"

	"github.com/juju/juju/mongo"
)

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
	connInfo
}

// NewDumper returns a new value with a Dump method for dumping the
// juju state database.
func NewDumper(info connInfo) *mongoDumper {
	return &mongoDumper{info}
}

// Dump dumps the juju state database.
func (md *mongoDumper) Dump(dumpDir string) error {
	address, username, password, err := md.connInfo.Check()
	if err != nil {
		return errors.Trace(err)
	}

	mongodumpPath, err := getMongodumpPath()
	if err != nil {
		return errors.Annotate(err, "mongodump not available")
	}

	err = runCommand(
		mongodumpPath,
		"--oplog",
		"--ssl",
		"--host", address,
		"--username", username,
		"--password", password,
		"--out", dumpDir,
	)
	if err != nil {
		return errors.Annotate(err, "error dumping database")
	}

	return nil
}
