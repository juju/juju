// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

import (
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/loggo"
	"github.com/juju/utils/hash"
	"github.com/juju/utils/tar"

	"github.com/juju/juju/version"
)

var (
	logger = loggo.GetLogger("juju.state.backup")
	sep    = string(os.PathSeparator)
)

type BackupCreator interface {
	Prepare() error
	CleanUp() error
	Run(dbinfo *DBConnInfo) (string, io.ReadCloser, int64, error)
}

type newBackup struct {
	tempdir  string
	bkpdir   string
	dumpdir  string
	tempfile string
	archive  io.WriteCloser
	rootfile io.WriteCloser
}

func (b *newBackup) Prepare() error {
	tempDir, err := ioutil.TempDir("", "jujuBackup")
	bkpDir := filepath.Join(tempDir, "juju-backup")
	dumpDir := filepath.Join(bkpDir, "dump")
	tempfile := filepath.Join(tempDir, "backup.tar.gz")
	err = os.MkdirAll(dumpDir, os.FileMode(0755))
	if err != nil {
		return fmt.Errorf("error creating backup temporary directory: %v", err)
	}
	archive, err := os.Create(b.tempfile)
	if err != nil {
		return fmt.Errorf("error creating backup archive file: %v", err)
	}
	rootfile, err := os.Create(filepath.Join(bkpDir, "root.tar"))
	if err != nil {
		return fmt.Errorf("error creating root archive file: %v", err)
	}

	b.tempdir = tempDir
	b.bkpdir = bkpDir
	b.dumpdir = dumpDir
	b.tempfile = tempfile
	b.rootfile = rootfile
	b.archive = archive
	return nil
}

func (b *newBackup) CleanUp() error {
	if b.tempdir != "" {
		err := os.RemoveAll(b.tempdir)
		if err != nil {
			return fmt.Errorf("error deleting temp directory: %v", err)
		}
		b.tempdir = ""
	}
	if b.rootfile != nil {
		err := b.rootfile.Close()
		if err != nil {
			return fmt.Errorf("error closing root file: %v", err)
		}
		b.rootfile = nil
	}
	if b.archive != nil {
		err := b.archive.Close()
		if err != nil {
			return fmt.Errorf("error closing archive file: %v", err)
		}
		b.archive = nil
	}
	return nil
}

func (b *newBackup) Run(dbinfo *DBConnInfo) (string, io.ReadCloser, int64, error) {
	// Bundle up the state-related files.
	filenames, err := getFilesToBackup()
	if err != nil {
		return "", nil, 0, fmt.Errorf("error building state file list: %v", err)
	}
	_, err = tar.TarFiles(filenames, b.rootfile, sep)
	if err != nil {
		return "", nil, 0, fmt.Errorf("error bundling state files: %v", err)
	}
	err = b.rootfile.Close()
	if err != nil {
		return "", nil, 0, fmt.Errorf("error closing root file: %v", err)
	}
	b.rootfile = nil

	// Dump the database.
	err = dumpDatabase(dbinfo, b.dumpdir)
	if err != nil {
		return "", nil, 0, fmt.Errorf("error dumping database: %v", err)
	}

	// Build the final tarball.
	hashed := hash.NewSHA1Proxy(b.archive)
	tarball := gzip.NewWriter(hashed)
	_, err = tar.TarFiles([]string{b.bkpdir}, tarball, b.tempdir)
	if err != nil {
		return "", nil, 0, fmt.Errorf("error bundling final archive", err)
	}
	err = tarball.Close()
	if err != nil {
		return "", nil, 0, fmt.Errorf("error closing gzip writer: %v", err)
	}
	err = b.archive.Close()
	if err != nil {
		return "", nil, 0, fmt.Errorf("error closing archive file: %v", err)
	}
	b.archive = nil
	checksum := hashed.Hash()

	// Re-open the archive for reading.
	archive, err := os.Open(b.tempfile)
	if err != nil {
		return "", nil, 0, fmt.Errorf("error opening temp archive: %v", err)
	}
	stat, err := archive.Stat()
	if err != nil {
		return "", nil, 0, fmt.Errorf("error reading file info: %v", err)
	}
	size := stat.Size()

	return checksum, archive, size, nil
}

// CreateBackup creates a new backup archive (.tar.gz) containing the
// data necessary to restore the juju state.  This archive is stored on
// the state server for later retrieval or use.
//
// The archive contains a dump folder with the output of mongodump
// command and a root.tar file which contains all the juju state-related
// files.
func CreateBackup(
	dbinfo *DBConnInfo, stor BackupStorage, name string, creator BackupCreator,
) (*BackupInfo, error) {
	if creator == nil {
		creator = &newBackup{}
		defer creator.CleanUp()
		err := creator.Prepare()
		if err != nil {
			return nil, fmt.Errorf("error preparing for backup: %v", err)
		}
	}

	// Create the info.
	info := BackupInfo{
		Name:      name,
		Timestamp: time.Now().UTC(),
		Version:   version.Current.Number,
		Status:    StatusBuilding,
	}
	err := stor.Add(&info, nil)
	if err != nil {
		return nil, fmt.Errorf("error initializing backup info: %v", err)
	}

	// Create the archive.
	checksum, archive, size, err := creator.Run(dbinfo)
	if err != nil {
		info.Status = StatusFailed
		stor.Add(&info, nil)
		return nil, fmt.Errorf("backup failed: %v", err)
	}
	defer archive.Close()

	// Update the info.
	info.CheckSum = checksum
	info.Size = size
	info.Status = StatusStoring

	// Store the archive.
	err = stor.Add(&info, archive)
	if err != nil {
		info.Status = StatusFailed
		stor.Add(&info, nil)
		return nil, fmt.Errorf("error while storing backup archive: %v", err)
	}
	info.Status = StatusAvailable
	stor.Add(&info, nil)

	return &info, nil
}
