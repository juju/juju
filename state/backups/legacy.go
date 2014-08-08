// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/hash"
	"github.com/juju/utils/tar"
)

// Backup creates a tar.gz file named juju-backup_<date YYYYMMDDHHMMSS>.tar.gz
// in the specified outputFolder.
// The backup contents look like this:
//   juju-backup/dump/ - the files generated from dumping the database
//   juju-backup/root.tar - contains all the files needed by juju
// Between the two, this is all that is necessary to later restore the
// juju agent on another machine.
func Backup(password string, username string, outputFolder string, addr string) (filename string, sha1sum string, err error) {
	// YYYYMMDDHHMMSS
	formattedDate := time.Now().Format("20060102150405")
	bkpFile := fmt.Sprintf("juju-backup_%s.tar.gz", formattedDate)

	// Prepare the temp dirs.
	root, contentdir, dumpdir, err := prepareTemp()
	if err != nil {
		return "", "", errors.Trace(err)
	}
	defer os.RemoveAll(root)

	// Dump the files.
	logger.Debugf("dumping state-related files")
	err = dumpFiles(contentdir)
	if err != nil {
		return "", "", errors.Trace(err)
	}

	// Dump the database.
	logger.Debugf("dumping database")
	dbinfo := NewDBConnInfo(addr, username, password)
	err = dumpDatabase(dbinfo, dumpdir)
	if err != nil {
		return "", "", errors.Trace(err)
	}

	// Bundle it all into a tarball.
	logger.Debugf("building archive file")
	shaSum, err := createBundle(bkpFile, outputFolder, contentdir, root+sep)
	if err != nil {
		return "", "", errors.Trace(err)
	}

	return bkpFile, shaSum, nil
}

func prepareTemp() (root, contentdir, dumpdir string, err error) {
	root, err = ioutil.TempDir("", "jujuBackup")
	contentdir = filepath.Join(root, "juju-backup")
	dumpdir = filepath.Join(contentdir, "dump")
	err = os.MkdirAll(dumpdir, os.FileMode(0755))
	if err != nil {
		err = errors.Annotate(err, "error creating temporary directories")
	}
	return
}

func createBundle(name, outdir, contentdir, root string) (string, error) {
	archive, err := os.Create(filepath.Join(outdir, name))
	if err != nil {
		return "", errors.Annotate(err, "error opening archive file")
	}
	defer archive.Close()
	hasher := hash.NewSHA1Proxy(archive)
	tarball := gzip.NewWriter(hasher)

	_, err = tar.TarFiles([]string{contentdir}, tarball, root)
	tarball.Close()
	if err != nil {
		return "", errors.Annotate(err, "error bundling final archive")
	}

	return hasher.Base64Sum(), nil
}

// StorageName returns the path in environment storage where a backup
// should be stored.  That name is derived from the provided filename.
func StorageName(filename string) string {
	// Use of path.Join instead of filepath.Join is intentional - this
	// is an environment storage path not a filesystem path.
	return path.Join("/backups", filepath.Base(filename))
}
