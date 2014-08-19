// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/juju/errors"
)

// Backup creates a tar.gz file named juju-backup_<date YYYYMMDDHHMMSS>.tar.gz
// in the specified outputFolder.
// The backup contents look like this:
//   juju-backup/dump/ - the files generated from dumping the database
//   juju-backup/root.tar - contains all the files needed by juju
// Between the two, this is all that is necessary to later restore the
// juju agent on another machine.
func Backup(password string, username string, outputFolder string, addr string) (filename string, sha1sum string, err error) {
	// Call create().
	db := mongoDumper{addr, username, password}
	filesToBackUp, err := getFilesToBackup("")
	if err != nil {
		return "", "", errors.Trace(err)
	}
	args := createArgs{filesToBackUp, &db}
	result, err := create(&args)
	if err != nil {
		return "", "", errors.Trace(err)
	}

	// Copy the archive file.
	formattedDate := time.Now().Format("20060102150405") // YYYYMMDDHHMMSS
	bkpFile := fmt.Sprintf("juju-backup_%s.tar.gz", formattedDate)

	file, err := os.Create(filepath.Join(outputFolder, bkpFile))
	if err != nil {
		return "", "", errors.Annotate(err, "error creating archive file")
	}
	_, err = io.Copy(file, result.archiveFile)
	if err != nil {
		return "", "", errors.Annotate(err, "error saving archive file")
	}

	// Return the info.
	return bkpFile, result.checksum, nil
}

// StorageName returns the path in environment storage where a backup
// should be stored.  That name is derived from the provided filename.
func StorageName(filename string) string {
	// Use of path.Join instead of filepath.Join is intentional - this
	// is an environment storage path not a filesystem path.
	return path.Join("/backups", filepath.Base(filename))
}
