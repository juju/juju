// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"compress/gzip"
	"crypto/sha1"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/utils/hash"
	"github.com/juju/utils/tar"

	"github.com/juju/juju/state/backups/archive"
)

// TODO(ericsnow) One concern is files that get out of date by the time
// backup finishes running.  This is particularly a problem with log
// files.

const (
	tempPrefix   = "jujuBackup-"
	tempFilename = "juju-backup.tar.gz"
)

type dumper interface {
	Dump(dumpDir string) error
}

type createArgs struct {
	filesToBackUp []string
	db            dumper
}

type createResult struct {
	archiveFile io.ReadCloser
	size        int64
	checksum    string
}

// create builds a new backup archive file and returns it.  It also
// updates the metadata with the file info.
func create(args *createArgs) (*createResult, error) {
	// Prepare the backup builder.
	builder, err := newBuilder(args.filesToBackUp, args.db)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer builder.cleanUp()

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
	db dumper
	// archiveFile is the backup archive file.
	archiveFile *os.File
	// bundleFile is the inner archive file containing all the juju
	// state-related files gathered during backup.
	bundleFile *os.File
}

// newBuilder returns a new backup archive builder.  It creates the temp
// directories which backup uses as its staging area while building the
// archive.  It also creates the archive
// (temp root, tarball root, DB dumpdir), along with any error.
func newBuilder(filesToBackUp []string, db dumper) (*builder, error) {
	b := builder{
		filesToBackUp: filesToBackUp,
		db:            db,
	}

	// Create the backups workspace root directory.
	rootDir, err := ioutil.TempDir("", tempPrefix)
	if err != nil {
		return nil, errors.Annotate(err, "error making backups workspace")
	}
	filename := filepath.Join(rootDir, tempFilename)
	b.archive = &archive.Archive{filename, rootDir}

	// Create all the direcories we need.  We go with user-only
	// permissions on principle; the directories are short-lived so in
	// practice it shouldn't matter much.
	err = os.MkdirAll(b.archive.DBDumpDir(), 0700)
	if err != nil {
		b.cleanUp()
		return nil, errors.Annotate(err, "error creating temp directories")
	}

	// Create the archive files.  We do so here to fail as early as
	// possible.
	b.archiveFile, err = os.Create(filename)
	if err != nil {
		b.cleanUp()
		return nil, errors.Annotate(err, "error creating archive file")
	}

	b.bundleFile, err = os.Create(b.archive.FilesBundle())
	if err != nil {
		b.cleanUp()
		return nil, errors.Annotate(err, `error creating bundle file`)
	}

	return &b, nil
}

func (b *builder) closeArchiveFile() error {
	if b.archiveFile == nil {
		return nil
	}

	if err := b.archiveFile.Close(); err != nil {
		return errors.Annotate(err, "error closing archive file")
	}

	b.archiveFile = nil
	return nil
}

func (b *builder) closeBundleFile() error {
	if b.bundleFile == nil {
		return nil
	}

	if err := b.bundleFile.Close(); err != nil {
		return errors.Annotate(err, `error closing "bundle" file`)
	}

	b.bundleFile = nil
	return nil
}

func (b *builder) removeRootDir() error {
	if b.archive == nil || b.archive.UnpackedRootDir == "" {
		return nil
	}

	if err := os.RemoveAll(b.archive.UnpackedRootDir); err != nil {
		return errors.Annotate(err, "error removing backups temp dir")
	}

	return nil
}

func (b *builder) cleanUp() error {
	var failed int

	funcs := [](func() error){
		b.closeBundleFile,
		b.closeArchiveFile,
		b.removeRootDir,
	}
	for _, cleanupFunc := range funcs {
		if err := cleanupFunc(); err != nil {
			logger.Errorf(err.Error())
			failed++
		}
	}

	if failed > 0 {
		return errors.Errorf("%d errors during cleanup (see logs)", failed)
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
		return errors.Annotate(err, "cannot backup configuration files")
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
		return errors.Annotate(err, "error dumping juju state database")
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
		return errors.Annotate(err, "error bundling final archive")
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
		return nil, errors.Annotate(err, "error opening archive file")
	}

	// Get the size.
	stat, err := file.Stat()
	if err != nil {
		return nil, errors.Annotate(err, "error reading archive file info")
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
