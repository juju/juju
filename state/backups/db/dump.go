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

// Dumper is any type that dumps something to a dump dir.
type Dumper interface {
	// Dump something to dumpDir.
	Dump(dumpDir string) error
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

type mongoDumper struct {
	ConnInfo
}

// NewDumper returns a new value with a Dump method for dumping the
// juju state database.
func NewDumper(info ConnInfo) Dumper {
	return &mongoDumper{info}
}

// Dump dumps the juju state database.
func (md *mongoDumper) Dump(dumpDir string) error {
	err := md.ConnInfo.Validate()
	if err != nil {
		return errors.Trace(err)
	}
	address, username, password := md.Address, md.Username, md.Password

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
