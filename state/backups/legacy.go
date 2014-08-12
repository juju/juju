// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"compress/gzip"
	"crypto/sha1"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/hash"
	"github.com/juju/utils/tar"

	"github.com/juju/juju/state/backups/config"
	coreutils "github.com/juju/juju/utils"
)

var sep = string(os.PathSeparator)

var (
	runCommand       = coreutils.RunCommand
	getFilesToBackup = func(config config.BackupsConfig) ([]string, error) {
		return config.FilesToBackUp()
	}
	getDumpCmd = func(
		config config.BackupsConfig, outdir string,
	) (string, []string, error) {
		return config.DBDump(outdir)
	}
)

// Backup creates a tar.gz file named juju-backup_<date YYYYMMDDHHMMSS>.tar.gz
// in the specified outputFolder.
// The backup contents look like this:
//   juju-backup/dump/ - the files generated from dumping the database
//   juju-backup/root.tar - contains all the files needed by juju
// Between the two, this is all that is necessary to later restore the
// juju agent on another machine.
func Backup(
	password string, username string, outputFolder string, addr string,
) (filename string, sha1sum string, err error) {
	// Set up the config.
	conf, err := config.NewBackupsConfigRaw(addr, username, password)
	if err != nil {
		return "", "", errors.Trace(err)
	}
	f, s, err := runBackup(conf, outputFolder)
	if err != nil {
		return "", "", errors.Trace(err)
	}
	return f, s, nil
}

func runBackup(
	config config.BackupsConfig, outputFolder string,
) (filename string, sha1sum string, err error) {
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
	logger.Infof("dumping state-related files")
	logger.Debugf("dumping state-related files")
	files, err := getFilesToBackup(config)
	if err != nil {
		return "", "", errors.Trace(err)
	}
	err = dumpFiles(files, contentdir)
	if err != nil {
		return "", "", errors.Trace(err)
	}

	// Dump the database.
	logger.Infof("dumping database")
	cmd, args, err := getDumpCmd(config, dumpdir)
	if err != nil {
		return "", "", errors.Trace(err)
	}
	err = runCommand(cmd, args...)
	if err != nil {
		return "", "", errors.Trace(err)
	}

	// Bundle it all into a tarball.
	logger.Infof("building archive file (%s)", bkpFile)
	// We add a trailing slash (or whatever) to root so that everything
	// in the path up to and including that slash is stripped off when
	// each file is added to the tar file.
	sep := string(os.PathSeparator)
	shaSum, err := createBundle(bkpFile, outputFolder, contentdir, root+sep)
	if err != nil {
		return "", "", errors.Trace(err)
	}

	return bkpFile, shaSum, nil
}

// prepareTemp creates the temp directories which backup uses as its
// staging area while building the archive.  It returns the paths,
// (temp root, tarball root, DB dumpdir), along with any error.
func prepareTemp() (string, string, string, error) {
	tempRoot, err := ioutil.TempDir("", "jujuBackup")
	if err != nil {
		err = errors.Annotate(err, "error creating root temp directory")
		return "", "", "", err
	}
	tarballRoot := filepath.Join(tempRoot, "juju-backup")
	dbDumpdir := filepath.Join(tarballRoot, "dump")
	// We go with user-only permissions on principle; the directories
	// are short-lived so in practice it shouldn't matter much.
	err = os.MkdirAll(dbDumpdir, os.FileMode(0700))
	if err != nil {
		err = errors.Annotate(err, "error creating temp directories")
		return "", "", "", err
	}
	return tempRoot, tarballRoot, dbDumpdir, nil
}

func dumpFiles(backupFiles []string, dumpdir string) error {
	tarFile, err := os.Create(filepath.Join(dumpdir, "root.tar"))
	if err != nil {
		return errors.Annotate(err, "error while opening initial archive")
	}
	defer tarFile.Close()

	_, err = tar.TarFiles(backupFiles, tarFile, sep)
	if err != nil {
		return errors.Annotate(err, "cannot backup configuration files")
	}
	return nil
}

func createBundle(name, outdir, contentdir, root string) (string, error) {
	archive, err := os.Create(filepath.Join(outdir, name))
	if err != nil {
		return "", errors.Annotate(err, "error opening archive file")
	}
	defer archive.Close()

	// Build the tarball, writing out to both the archive file and a
	// SHA1 hash.  The hash will correspond to the gzipped file rather
	// than to the uncompressed contents of the tarball.  This is so
	// that users can compare the published checksum against the
	// checksum of the file without having to decompress it first.
	hasher := hash.NewHashingWriter(archive, sha1.New())
	err = func() error {
		tarball := gzip.NewWriter(hasher)
		defer tarball.Close()

		_, err := tar.TarFiles([]string{contentdir}, tarball, root)
		return err
	}()
	if err != nil {
		return "", errors.Annotate(err, "error bundling final archive")
	}

	// Return the SHA1 checksum.
	// Gzip writers may buffer what they're writing so we must call
	// Close() on the writer *before* getting the checksum from the
	// hasher.
	return hasher.Base64Sum(), nil
}

// StorageName returns the path in environment storage where a backup
// should be stored.  That name is derived from the provided filename.
func StorageName(filename string) string {
	// Use of path.Join instead of filepath.Join is intentional - this
	// is an environment storage path not a filesystem path.
	return path.Join("/backups", filepath.Base(filename))
}
