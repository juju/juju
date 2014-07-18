// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/juju/loggo"
)

var (
	logger = loggo.GetLogger("juju.backup")
	sep    = string(os.PathSeparator)
)

// This is effectively the "connection string".
// It probably doesn't belong here in the long term.
type DBConnInfo struct {
	Hostname string
	Username string
	Password string
}

func bundleStateFiles(targetDir string) error {
	tarFile := filepath.Join(targetDir, "root.tar")
	backupFiles, err := getFilesToBackup()
	if err != nil {
		return fmt.Errorf("could not determine files to backup: %v", err)
	}

	ar := archive{backupFiles, sep}
	err = ar.Create(tarFile)
	if err != nil {
		return fmt.Errorf("could not back up state files: %v", err)
	}
	return nil
}

// Note that the returned hash is for the compressed data.
func writeTarball(ar *archive, file io.WriteCloser) (hash string, err error) {
	checkClose := func(w io.Closer) {
		if closeErr := w.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("error closing backup file: %v", closeErr)
		}
	}
	defer checkClose(file)

	// Prepare the hasher.
	proxy := newSHA1Proxy(file)

	// Write the archive.
	err = ar.WriteGzipped(proxy)
	if err != nil {
		return "", err
	}

	// Return the hash.
	hash = proxy.Hash()
	return hash, nil
}

// Backup creates a tar.gz file named juju-backup-<date YYYYMMDD-HHMMSS>.tar.gz
// in the specified outputFolder.
// The backup contains a dump folder with the output of mongodump command
// and a root.tar file which contains all the system files obtained from
// the output of getFilesToBackup.
func Backup(dbinfo *DBConnInfo, outputFolder string) (string, string, error) {
	// Prepare an empty file into which to store the backup.
	tail := sep
	if strings.HasSuffix(outputFolder, sep) {
		tail = ""
	}
	archivefile, filename, err := CreateEmptyFile(outputFolder+tail, false)
	if err != nil {
		return "", "", err
	}
	defer archivefile.Close() // just in case

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

	// Set up the archiver and the hasher.
	ar := archive{
		Files:       []string{bkpDir},
		StripPrefix: tempDir + sep,
	}

	// Write out the full backup tarball.
	logger.Infof("writing backup tarball: %s", filename)
	shaSum, err := writeTarball(&ar, archivefile)
	if err != nil {
		return "", "", fmt.Errorf("could not write out backup file: %v", err)
	}
	logger.Infof("backup tarball created")
	logger.Infof("backup tarball SHA-1 hash: %s", shaSum)

	return filepath.Base(filename), shaSum, nil
}

// StorageName returns the path in environment storage where a backup
// should be stored.
func StorageName(filename string) string {
	// Use of path.Join instead of filepath.Join is intentional - this
	// is an environment storage path not a filesystem path.
	return path.Join("/backups", filepath.Base(filename))
}
