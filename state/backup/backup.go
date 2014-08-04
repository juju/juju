// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/loggo"

	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/version"
)

var (
	logger = loggo.GetLogger("juju.state.backup")
	sep    = string(os.PathSeparator)
)

// CreateBackup creates a new backup archive.  If store is true, the
// backup is stored to environment storage.
// The backup contains a dump folder with the output of mongodump command
// and a root.tar file which contains all the system files obtained from
// the output of getFilesToBackup.
func CreateBackup(
	dbinfo *DBConnInfo, name string, storage storage.StorageWriter,
) (*BackupInfo, error) {
	logger.Infof("backing up juju state")
	info := BackupInfo{Name: name}

	archive, err := create(&info, dbinfo)
	if err != nil {
		return nil, err
	}
	defer archive.Close()
	logger.Infof("created: %q (SHA-1: %s)", info.Name, info.CheckSum)

	// Store the backup.
	err = store(&info, storage, archive)

	return &info, err
}

func create(info *BackupInfo, dbinfo *DBConnInfo) (_ io.ReadCloser, err error) {
	newbackup := newBackup{}
	cleanup := func() {
		nbErr := newbackup.cleanup()
		if nbErr != nil && err == nil {
			err = nbErr
		}
	}

	// Prepare the backup.
	err = newbackup.prepare()
	if err != nil {
		return nil, err
	}
	defer cleanup()

	// Run the backup.
	sha1sum, err := newbackup.run(dbinfo)
	if err != nil {
		return nil, err
	}

	// Set the backup fields.
	if info.Name == "" {
		info.Name = filepath.Base(newbackup.filename)
	}
	timestamp, tsErr := ExtractTimestamp(newbackup.filename)
	if tsErr != nil {
		timestamp = time.Now().UTC()
	}
	info.Timestamp = timestamp
	info.CheckSum = sha1sum
	info.Version = version.Current.Number

	// Open the archive (before we delete it).
	archive, err := os.Open(newbackup.filename)
	if err != nil {
		return nil, fmt.Errorf("error opening archive: %v", err)
	}
	finfo, err := archive.Stat()
	if err != nil {
		return nil, fmt.Errorf("error getting file info: %v", err)
	}
	info.Size = finfo.Size()

	return archive, err
}

func store(info *BackupInfo, storage storage.StorageWriter, archive io.Reader) error {
	logger.Debugf("storing %q", info.Name)

	err := storage.Put(info.Name, archive, info.Size)
	if err != nil {
		return fmt.Errorf("error storing archive: %v", err)
	}

	logger.Infof("stored: %q (SHA-1: %s)", info.Name, info.CheckSum)
	return nil
}

//---------------------------
// legacy API

type fileStore struct {
	dirname string
}

func (f *fileStore) Put(name string, r io.Reader, len int64) error {
	stored, err := os.Create(filepath.Join(f.dirname, name))
	if err != nil {
		return err
	}
	defer stored.Close()
	if _, err = io.Copy(stored, r); err != nil {
		return err
	}
	return nil
}

func (f *fileStore) Remove(name string) error {
	return nil
}

func (f *fileStore) RemoveAll() error {
	return nil
}

// XXX Remove!
func Backup(pw, tag, outputFolder, host string) (string, string, error) {
	dbinfo := DBConnInfo{
		Hostname: host,
		Username: tag,
		Password: pw,
	}
	stor := fileStore{outputFolder}
	backup, err := CreateBackup(&dbinfo, "", &stor)
	if err != nil {
		return "", "", err
	}
	return backup.Name, backup.CheckSum, nil
}
