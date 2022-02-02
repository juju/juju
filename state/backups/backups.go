// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

/*
Package backups contains all the stand-alone backup-related
functionality for juju state. That functionality is encapsulated by
the backups.Backups type. The package also exposes a few key helpers
and components.

Backups are not a part of juju state nor of normal state operations.
However, they certainly are tightly coupled with state (the very
subject of backups). This puts backups in an odd position, particularly
with regard to the storage of backup metadata and archives.

As noted above backups are about state but not a part of state. So
exposing backup-related methods on State would imply the wrong thing.
Thus most of the functionality here is defined at a high level without
relation to state. A few low-level parts or helpers are exposed as
functions to which you pass a state value. Those are kept to a minimum.

Note that state (and juju as a whole) currently does not have a
persistence layer abstraction to facilitate separating different
persistence needs and implementations. As a consequence, state's
data, whether about how an model should look or about existing
resources within an model, is dumped essentially straight into
State's mongo connection. The code in the state package does not
make any distinction between the two (nor does the package clearly
distinguish between state-related abstractions and state-related
data).

Backups add yet another category, merely taking advantage of State's
mongo for storage. In the interest of making the distinction clear,
among other reasons, backups uses its own database under state's mongo
connection.
*/
package backups

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/v3/du"
)

const (
	// FilenamePrefix is the prefix used for backup archive files.
	FilenamePrefix = "juju-backup-"

	// FilenameTemplate is used with time.Time.Format to generate a filename.
	FilenameTemplate = FilenamePrefix + "20060102-150405.tar.gz"
)

var logger = loggo.GetLogger("juju.state.backups")

var (
	getFilesToBackUp = GetFilesToBackUp
	getDBDumper      = NewDBDumper
	runCreate        = create
	finishMeta       = func(meta *Metadata, result *createResult) error {
		return meta.MarkComplete(result.size, result.checksum)
	}
	availableDisk = func(path string) uint64 {
		return du.NewDiskUsage(path).Available()
	}
	totalDisk = func(path string) uint64 {
		return du.NewDiskUsage(path).Size()
	}
	dirSize = totalDirSize
)

// Backups is an abstraction around all juju backup-related functionality.
type Backups interface {
	// Create creates a new juju backup archive. It updates
	// the provided metadata.
	Create(meta *Metadata, paths *Paths, dbInfo *DBInfo) (string, error)

	// Get returns the metadata and specified archive file.
	Get(fileName string) (*Metadata, io.ReadCloser, error)
}

type backups struct{}

// NewBackups creates a new Backups value using the FileStorage provided.
func NewBackups() Backups {
	return &backups{}
}

func totalDirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return err
	})
	return size, err
}

// Create creates and stores a new juju backup archive (based on arguments)
// and updates the provided metadata.  A filename to download the backup is provided.
func (b *backups) Create(meta *Metadata, paths *Paths, dbInfo *DBInfo) (string, error) {
	// TODO(fwereade): 2016-03-17 lp:1558657
	meta.Started = time.Now().UTC()

	// The metadata file will not contain the ID or the "finished" data.
	// However, that information is not as critical. The alternatives
	// are either adding the metadata file to the archive after the fact
	// or adding placeholders here for the finished data and filling
	// them in afterward.  Neither is particularly trivial.
	metadataFile, err := meta.AsJSONBuffer()
	if err != nil {
		return "", errors.Annotate(err, "while preparing the metadata")
	}

	// Create the archive.
	filesToBackUp, err := getFilesToBackUp("", paths)
	if err != nil {
		return "", errors.Annotate(err, "while listing files to back up")
	}

	var totalFileSizes int64
	for _, f := range filesToBackUp {
		size, err := dirSize(f)
		if err != nil {
			return "", errors.Trace(err)
		}
		totalFileSizes += size
	}

	totalFizeSizesMiB := int64(dbInfo.ApproxSizeMB) + totalFileSizes/humanize.MiByte
	logger.Infof("backing up %dMiB (files) and %dMiB (database) = %dMiB",
		totalFizeSizesMiB, dbInfo.ApproxSizeMB, int(totalFizeSizesMiB)+dbInfo.ApproxSizeMB)

	destinationDir := paths.BackupDir
	if destinationDir == "" {
		destinationDir = os.TempDir()
	}

	if _, err := os.Stat(destinationDir); err != nil {
		if os.IsNotExist(err) {
			return "", errors.Errorf("backup destination directory %q does not exist", destinationDir)
		}
		return "", errors.NewNotValid(nil, fmt.Sprintf("invalid backup destination directory %q: %v", destinationDir, err))
	}
	if !filepath.IsAbs(destinationDir) {
		return "", errors.Errorf("cannot use relative backup destination directory %q", destinationDir)
	}

	// We require space equal to the larger of:
	// - smaller of 5GB or 10% of the total disk size
	// - 20% of the backup size
	// on top of the approximate backup size to be available.
	const minFreeAbsolute = 5 * humanize.GiByte

	diskSizeMargin := float64(totalDisk(destinationDir)) * 0.10
	if diskSizeMargin > minFreeAbsolute {
		diskSizeMargin = minFreeAbsolute
	}
	backupSizeMargin := float64(totalFizeSizesMiB) * 0.20 * humanize.MiByte
	if backupSizeMargin < diskSizeMargin {
		backupSizeMargin = diskSizeMargin
	}
	wantFree := uint64(totalFizeSizesMiB) + uint64(backupSizeMargin/humanize.MiByte)

	available := availableDisk(destinationDir) / humanize.MiByte
	logger.Infof("free disk on volume hosting %q: %dMiB", destinationDir, available)
	if available < wantFree {
		return "", errors.Errorf("not enough free space in %q; want %dMiB, have %dMiB", destinationDir, wantFree, available)
	}

	dumper, err := getDBDumper(dbInfo)
	if err != nil {
		return "", errors.Annotate(err, "while preparing for DB dump")
	}

	args := createArgs{
		destinationDir: destinationDir,
		filesToBackUp:  filesToBackUp,
		db:             dumper,
		metadataReader: metadataFile,
	}
	result, err := runCreate(&args)
	if err != nil {
		return "", errors.Annotate(err, "while creating backup archive")
	}
	defer func() { _ = result.archiveFile.Close() }()

	// Finalize the metadata.
	err = finishMeta(meta, result)
	if err != nil {
		return "", errors.Annotate(err, "while updating metadata")
	}

	return result.filename, nil
}

// Get retrieves the associated metadata and archive file a file on the machine.
func (b *backups) Get(fileName string) (_ *Metadata, _ io.ReadCloser, err error) {
	defer func() {
		// On success, remove the retrieved file.
		if err != nil {
			return
		}
		if err2 := os.Remove(fileName); err2 != nil && !os.IsNotExist(err2) {
			logger.Errorf("error removing backup archive: %v", err2.Error())
		}
	}()

	readCloser, err := os.Open(fileName)
	if err != nil {
		return nil, nil, errors.Annotate(err, "while opening archive file for download")
	}

	meta, err := BuildMetadata(readCloser)
	if err != nil {
		return nil, nil, errors.Annotate(err, "while creating metadata for archive file to download")
	}

	// BuildMetadata copied readCloser, so reset handle to beginning of the file
	_, err = readCloser.Seek(0, io.SeekStart)
	if err != nil {
		return nil, nil, errors.Annotate(err, "while resetting archive file to download")
	}

	return meta, readCloser, nil
}
