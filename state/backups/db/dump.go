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
	Info
	// binPath is the path to the dump executable.
	binPath string
}

// NewDumper returns a new value with a Dump method for dumping the
// juju state database.
func NewDumper(info Info) (Dumper, error) {
	err := info.Validate()
	if err != nil {
		return nil, errors.Trace(err)
	}

	mongodumpPath, err := getMongodumpPath()
	if err != nil {
		return nil, errors.Annotate(err, "mongodump not available")
	}

	dumper := mongoDumper{
		Info:    info,
		binPath: mongodumpPath,
	}
	return &dumper, nil
}

func (md *mongoDumper) options(dumpDir string, dbName string) []string {
	if dbName != "" {
		dumpDir = filepath.Join(dumpDir, dbName)
	}

	options := []string{
		"--ssl",
		"--authenticationDatabase", "admin",
		"--host", md.Address,
		"--username", md.Username,
		"--password", md.Password,
		"--out", dumpDir,
	}

	if dbName == "" {
		options = append(options, "--oplog")
	} else {
		options = append(options, "--db", dbName)
	}

	return options
}

func (md *mongoDumper) dump(dumpDir, dbName string) error {
	options := md.options(dumpDir, dbName)
	if err := runCommand(md.binPath, options...); err != nil {
		suffix := " (" + dbName + ")"
		if dbName == "" {
			suffix = "s"
		}
		return errors.Annotatef(err, "error dumping database%s", suffix)
	}
	return nil
}

func (md *mongoDumper) strip(dumpDir, dbNames ...string) error {

}

// Dump dumps the juju state database.
func (md *mongoDumper) Dump(baseDumpDir string) error {
	if err := md.dump(baseDumpDir, dbName); err != nil {
		return errors.Trace(err)
	}

	var ignored []string

	return nil
}

func listDatabases(dumpDir string) ([]string, error) {
	list, err := ioutil.ReadDir(dumpDir)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var databases []string
	for _, info := range list {
		if !info.IsDir() {
			// This will include oplog.bson.
			continue
		}
		databases = append(databases, info.Name())
	}
	return databases, nil
}
