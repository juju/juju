// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"compress/gzip"
	"crypto/sha1"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/hash"
	"github.com/juju/utils/tar"

	"github.com/juju/juju/state/backups/archive"
	"github.com/juju/juju/state/backups/db"
)

// TODO(ericsnow) One concern is files that get out of date by the time
// backup finishes running.  This is particularly a problem with log
// files.

const (
	tempPrefix   = "jujuBackup-"
	tempFilename = "juju-backup.tar.gz"
)

type createArgs struct {
	filesToBackUp []string
	db            db.Dumper
}

type createResult struct {
	archiveFile io.ReadCloser
	size        int64
	checksum    string
}

// create builds a new backup archive file and returns it.  It also
// updates the metadata with the file info.
func create(args *createArgs) (_ *createResult, err error) {
	// Prepare the backup builder.
	builder, err := newBuilder(args.filesToBackUp, args.db)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer func() {
		if cerr := builder.cleanUp(); cerr != nil {
			cerr.Log(logger)
			if err == nil {
				err = cerr
			}
		}
	}()

	// Build the backup.
	if err := builder.buildAll(); err != nil {
		return nil, errors.Trace(err)
	}

	// Get the result.
	result, err := builder.result()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Return the result.  Note that the entire build workspace will be
	// deleted at the end of this function.  This includes the backup
	// archive file we built.  However, the handle to that file in the
	// result will still be open and readable.
	// If we ever support state machines on Windows, this will need to
	// change (you can't delete open files on Windows).
	return result, nil
}

// builder exposes the machinery for creating a backup of juju's state.
type builder struct {
	// archive is the backups archive summary.
	archive *archive.Archive
	// checksum is the checksum of the archive file.
	checksum string
	// filesToBackUp is the paths to every file to include in the archive.
	filesToBackUp []string
	// db is the wrapper around the DB dump command and args.
	db db.Dumper
	// archiveFile is the backup archive file.
	archiveFile io.WriteCloser
	// bundleFile is the inner archive file containing all the juju
	// state-related files gathered during backup.
	bundleFile io.WriteCloser
}

// newBuilder returns a new backup archive builder.  It creates the temp
// directories which backup uses as its staging area while building the
// archive.  It also creates the archive
// (temp root, tarball root, DB dumpdir), along with any error.
func newBuilder(filesToBackUp []string, db db.Dumper) (_ *builder, err error) {
	b := builder{
		filesToBackUp: filesToBackUp,
		db:            db,
	}
	defer func() {
		if err != nil {
			if cerr := b.cleanUp(); cerr != nil {
				cerr.Log(logger)
			}
		}
	}()

	// Create the backups workspace root directory.
	rootDir, err := ioutil.TempDir("", tempPrefix)
	if err != nil {
		return nil, errors.Annotate(err, "while making backups workspace")
	}
	filename := filepath.Join(rootDir, tempFilename)
	b.archive = &archive.Archive{filename, rootDir}

	// Create all the direcories we need.  We go with user-only
	// permissions on principle; the directories are short-lived so in
	// practice it shouldn't matter much.
	err = os.MkdirAll(b.archive.DBDumpDir(), 0700)
	if err != nil {
		return nil, errors.Annotate(err, "while creating temp directories")
	}

	// Create the archive files.  We do so here to fail as early as
	// possible.
	b.archiveFile, err = os.Create(filename)
	if err != nil {
		return nil, errors.Annotate(err, "while creating archive file")
	}

	b.bundleFile, err = os.Create(b.archive.FilesBundle())
	if err != nil {
		return nil, errors.Annotate(err, `while creating bundle file`)
	}

	return &b, nil
}

func (b *builder) closeArchiveFile() error {
	// Currently this method isn't thread-safe (doesn't need to be).
	if b.archiveFile == nil {
		return nil
	}

	if err := b.archiveFile.Close(); err != nil {
		return errors.Annotate(err, "while closing archive file")
	}

	b.archiveFile = nil
	return nil
}

func (b *builder) closeBundleFile() error {
	// Currently this method isn't thread-safe (doesn't need to be).
	if b.bundleFile == nil {
		return nil
	}

	if err := b.bundleFile.Close(); err != nil {
		return errors.Annotate(err, "while closing bundle file")
	}

	b.bundleFile = nil
	return nil
}

func (b *builder) removeRootDir() error {
	// Currently this method isn't thread-safe (doesn't need to be).
	if b.archive == nil || b.archive.UnpackedRootDir == "" {
		return nil
	}

	if err := os.RemoveAll(b.archive.UnpackedRootDir); err != nil {
		return errors.Annotate(err, "while removing backups temp dir")
	}

	return nil
}

type cleanupErrors struct {
	Errors []error
}

func (e cleanupErrors) Error() string {
	if len(e.Errors) == 1 {
		return fmt.Sprintf("while cleaning up: %v", e.Errors[0])
	} else {
		return fmt.Sprintf("%d errors during cleanup", len(e.Errors))
	}
}

func (e cleanupErrors) Log(logger loggo.Logger) {
	logger.Errorf(e.Error())
	for _, err := range e.Errors {
		logger.Errorf(err.Error())
	}
}

func (b *builder) cleanUp() *cleanupErrors {
	var errors []error

	if err := b.closeBundleFile(); err != nil {
		errors = append(errors, err)
	}
	if err := b.closeArchiveFile(); err != nil {
		errors = append(errors, err)
	}
	if err := b.removeRootDir(); err != nil {
		errors = append(errors, err)
	}

	if errors != nil {
		return &cleanupErrors{errors}
	}
	return nil
}

func (b *builder) buildFilesBundle() error {
	logger.Infof("dumping juju state-related files")
	if b.filesToBackUp == nil {
		logger.Infof("nothing to do")
		return nil
	}
	if b.bundleFile == nil {
		return errors.New("missing bundleFile")
	}

	stripPrefix := string(os.PathSeparator)
	_, err := tar.TarFiles(b.filesToBackUp, b.bundleFile, stripPrefix)
	if err != nil {
		return errors.Annotate(err, "while bundling state-critical files")
	}

	return nil
}

func (b *builder) buildDBDump() error {
	logger.Infof("dumping database")
	if b.db == nil {
		logger.Infof("nothing to do")
		return nil
	}

	dumpDir := b.archive.DBDumpDir()
	if err := b.db.Dump(dumpDir); err != nil {
		return errors.Annotate(err, "while dumping juju state database")
	}

	return nil
}

func (b *builder) buildArchive(outFile io.Writer) error {
	tarball := gzip.NewWriter(outFile)
	defer tarball.Close()

	// We add a trailing slash (or whatever) to root so that everything
	// in the path up to and including that slash is stripped off when
	// each file is added to the tar file.
	stripPrefix := b.archive.UnpackedRootDir + string(os.PathSeparator)
	filenames := []string{b.archive.ContentDir()}
	if _, err := tar.TarFiles(filenames, tarball, stripPrefix); err != nil {
		return errors.Annotate(err, "while bundling final archive")
	}

	return nil
}

func (b *builder) buildArchiveAndChecksum() error {
	logger.Infof("building archive file (%s)", b.archive.Filename)
	if b.archiveFile == nil {
		return errors.New("missing archiveFile")
	}

	// Build the tarball, writing out to both the archive file and a
	// SHA1 hash.  The hash will correspond to the gzipped file rather
	// than to the uncompressed contents of the tarball.  This is so
	// that users can compare the published checksum against the
	// checksum of the file without having to decompress it first.
	hasher := hash.NewHashingWriter(b.archiveFile, sha1.New())
	if err := b.buildArchive(hasher); err != nil {
		return errors.Trace(err)
	}

	// Save the SHA1 checksum.
	// Gzip writers may buffer what they're writing so we must call
	// Close() on the writer *before* getting the checksum from the
	// hasher.
	b.checksum = hasher.Base64Sum()

	return nil
}

func (b *builder) buildAll() error {
	// Dump the files.
	if err := b.buildFilesBundle(); err != nil {
		return errors.Trace(err)
	}

	// Dump the database.
	if err := b.buildDBDump(); err != nil {
		return errors.Trace(err)
	}

	// Bundle it all into a tarball.
	if err := b.buildArchiveAndChecksum(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (b *builder) result() (*createResult, error) {
	// Open the file in read-only mode.
	file, err := os.Open(b.archive.Filename)
	if err != nil {
		return nil, errors.Annotate(err, "while opening archive file")
	}

	// Get the size.
	stat, err := file.Stat()
	if err != nil {
		return nil, errors.Annotate(err, "while reading archive file info")
	}
	size := stat.Size()

	// Get the checksum.
	checksum := b.checksum

	// Return the result.
	result := createResult{
		archiveFile: file,
		size:        size,
		checksum:    checksum,
	}
	return &result, nil
}
