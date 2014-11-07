// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package db

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils"

	"github.com/juju/juju/agent"
)

// runExternalCommand it is intended to be a wrapper around utils.RunCommand
// its main objective is to provide a suiteable output for our use case.
func runExternalCommand(cmd string, args ...string) error {
	if out, err := utils.RunCommand(cmd, args...); err != nil {
		if _, ok := err.(*exec.ExitError); ok && len(out) > 0 {
			return errors.Annotatef(err, "error executing %q: %s", cmd, strings.Replace(string(out), "\n", "; ", -1))
		}
		return errors.Annotatef(err, "cannot execute %q", cmd)
	}
	return nil
}

// mongorestorePath will look for mongorestore binary on the system
// and return it if mongorestore actually exists.
// it will look first for the juju provided one and if not found make a
// try at a system one.
func mongorestorePath() (string, error) {
	const mongoRestoreFullPath string = "/usr/lib/juju/bin/mongorestore"

	if _, err := os.Stat(mongoRestoreFullPath); err == nil {
		return mongoRestoreFullPath, nil
	}

	path, err := exec.LookPath("mongorestore")
	if err != nil {
		return "", errors.Trace(err)
	}
	return path, nil
}

// mongoRestoreArgsForVersion returns a string slice containing the args to be used
// to call mongo restore since these can change depending on the backup method.
// Version 0: a dump made with --db, stoping the state server.
// Version 1: a dump made with --oplog with a running state server.
// TODO (perrito666) change versions to use metadata version
func mongoRestoreArgsForVersion(version int, dumpPath string) ([]string, error) {
	MGORestoreVersions := map[int][]string{}
	dbDir := filepath.Join(agent.DefaultDataDir, "db")

	MGORestoreVersions[0] = []string{
		"--drop",
		"--dbpath", dbDir,
		dumpPath}

	MGORestoreVersions[1] = []string{
		"--drop",
		"--oplogReplay",
		"--dbpath", dbDir,
		dumpPath}
	if restoreCommand, ok := MGORestoreVersions[version]; ok {
		return restoreCommand, nil
	}
	return nil, errors.Errorf("this backup file is incompatible with the current version of juju")
}

// PlaceNewMongo tries to use mongorestore to replace an existing
// mongo with the dump in newMongoDumpPath returns an error if its not possible.
func PlaceNewMongo(newMongoDumpPath string, version int) error {
	mongoRestore, err := mongorestorePath()
	if err != nil {
		return errors.Annotate(err, "mongorestore not available")
	}

	mgoRestoreArgs, err := mongoRestoreArgsForVersion(version, newMongoDumpPath)
	if err != nil {
		return errors.Errorf("cannot restore this backup version")
	}
	if err = runExternalCommand(
		"initctl",
		"stop",
		"juju-db"); err != nil {
		return errors.Annotate(err, "failed to stop mongo")
	}

	err = runExternalCommand(mongoRestore, mgoRestoreArgs...)

	if err != nil {
		return errors.Annotate(err, "failed to restore database dump")
	}

	if err = runExternalCommand(
		"initctl",
		"start",
		"juju-db"); err != nil {
		return errors.Annotate(err, "failed to start mongo")
	}

	return nil
}


