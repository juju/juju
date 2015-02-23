// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

import (
	"os"
	"path"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/utils"
)

const (
	// SharedSecretFile is the name of the Mongo shared secret file
	// located within the Juju data directory. It is used for replica
	// sets.
	SharedSecretFile = "shared-secret"

	dataDirName = "db"
)

var (
	configPath = "/etc/default/mongodb"
)

// DBDir returns the directory where mongod data should be stored
// relative to the provided directory.
func DBDir(rootDir string) string {
	return filepath.Join(rootDir, dataDirName)
}

func makeDBDir(dbDir string) error {
	if err := os.MkdirAll(dbDir, 0700); err != nil {
		return errors.Annotate(err, "cannot create mongo database directory")
	}
	return nil
}

func makeJournalDirs(dataDir string) error {
	journalDir := path.Join(dataDir, "journal")
	if err := os.MkdirAll(journalDir, 0700); err != nil {
		logger.Errorf("failed to make mongo journal dir %s: %v", journalDir, err)
		return err
	}

	// Manually create the prealloc files, since otherwise they get
	// created as 100M files. We create three files of 1MB each.
	prefix := filepath.Join(journalDir, "prealloc.")
	preallocSize := 1024 * 1024
	return preallocFiles(prefix, preallocSize, preallocSize, preallocSize)
}

func sslKeyPath(dataDir string) string {
	return filepath.Join(dataDir, "server.pem")
}

func sharedSecretPath(dataDir string) string {
	return filepath.Join(dataDir, SharedSecretFile)
}

func writeSSL(dataDir string, ssl SSLInfo) error {
	err := utils.AtomicWriteFile(
		sslKeyPath(dataDir),
		[]byte(ssl.CertKey()),
		0600,
	)
	if err != nil {
		return errors.Annotate(err, "cannot write SSL key")
	}

	err = utils.AtomicWriteFile(
		sharedSecretPath(dataDir),
		[]byte(ssl.SharedSecret),
		0600,
	)
	if err != nil {
		return errors.Annotate(err, "cannot write mongod shared secret")
	}

	return nil
}

func writeConf(content string) error {
	err := utils.AtomicWriteFile(
		configPath,
		[]byte(content),
		0644,
	)
	return errors.Trace(err)
}
