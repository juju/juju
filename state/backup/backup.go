// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/juju/loggo"
)

var (
	logger = loggo.GetLogger("juju.backup")
	sep    = string(os.PathSeparator)
)

// This is effectively the "connection string".
type dbConnInfo struct {
	Hostname string
	Username string
	Password string
}

// Backup creates a tar.gz file named juju-backup_<date YYYYMMDDHHMMSS>.tar.gz
// in the specified outputFolder.
// The backup contains a dump folder with the output of mongodump command
// and a root.tar file which contains all the system files obtained from
// the output of getFilesToBackup
func Backup(password string, username string, outputFolder string, addr string) (string, string, error) {
	dbinfo := dbConnInfo{
		Hostname: addr,
		Username: username,
		Password: password,
	}
	return backup(&dbinfo, outputFolder)
}

func bundleStateFiles(targetDir string) error {
	tarFile := filepath.Join(targetDir, "root.tar")
	backupFiles, err := getFilesToBackup()
	if err != nil {
		return fmt.Errorf("could not determine files to backup: %v", err)
	}
	_, err = tarFiles(backupFiles, tarFile, sep, false)
	if err != nil {
		return fmt.Errorf("could not back up configuration files: %v", err)
	}
	return nil
}

func backup(dbinfo *dbConnInfo, outputFolder string) (string, string, error) {
	// YYYYMMDDHHMMSS
	formattedDate := time.Now().Format("20060102150405")

	bkpFile := fmt.Sprintf("juju-backup_%s.tar.gz", formattedDate)

	// Prepare the temp directories.
	var bkpDir, dumpDir string
	tempDir, err := ioutil.TempDir("", "jujuBackup")
	if err == nil {
		defer os.RemoveAll(tempDir)
		logger.Debugf("backup temp dir: %s", tempDir)
		bkpDir = filepath.Join(tempDir, "juju-backup")
		dumpDir = filepath.Join(bkpDir, "dump")
		err = os.MkdirAll(dumpDir, os.FileMode(0755))
	}
	if err != nil {
		return "", "", fmt.Errorf("could not create backup temporary directory: %v", err)
	}

	// Dump the database.
	err = dumpDatabase(dbinfo, dumpDir)
	if err != nil {
		return "", "", err
	}

	// Bundle the state config and log files.
	err = bundleStateFiles(bkpDir)
	if err != nil {
		return "", "", err
	}

	// Create a new tarball containing the previous tarfile and the DB dump.
	target := filepath.Join(outputFolder, bkpFile)
	logger.Infof("creating backup tarball: %s", target)
	strip := tempDir + sep
	shaSum, err := tarFiles([]string{bkpDir}, target, strip, true)
	if err != nil {
		return "", "", fmt.Errorf("could not write out complete backup file: %v", err)
	}
	logger.Infof("backup tarball created")
	logger.Infof("backup tarball SHA-1 hash: %s", shaSum)

	return bkpFile, shaSum, nil
}

// StorageName returns the path in environment storage where a backup
// should be stored.
func StorageName(filename string) string {
	// Use of path.Join instead of filepath.Join is intentional - this
	// is an environment storage path not a filesystem path.
	return path.Join("/backups", filepath.Base(filename))
}
