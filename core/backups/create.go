// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"compress/gzip"
	"crypto/sha1"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/juju/utils/v4/hash"
	"github.com/juju/utils/v4/tar"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

// tempPrefix is the prefix used for the backup staging directories.
const tempPrefix = "jujuBackup-"

// DumpEntry is a single database dump file to include in the backup
// archive, under the archive's dump directory.
type DumpEntry struct {
	// Name is the path of the entry relative to the archive's dump
	// directory, e.g. "controller.yaml" or "models/<uuid>.yaml".
	Name string

	// Reader provides the contents of the dump entry.
	Reader io.Reader
}

// CreateArgs holds the arguments for building a backup archive.
type CreateArgs struct {
	// DestinationDir is the absolute path to the directory in which
	// the archive is stored. The staging area is created there too.
	DestinationDir string

	// FilesToBackUp is the list of absolute paths to the files to
	// bundle into the archive's root.tar.
	FilesToBackUp []string

	// DumpEntries are the database dumps written under the archive's
	// dump directory.
	DumpEntries []DumpEntry
}

// Create builds a new backup archive file in args.DestinationDir,
// named after meta.Started using FilenameTemplate. It updates the
// metadata with the file info and returns the archive filename.
func Create(meta *Metadata, args CreateArgs) (string, error) {
	if err := checkDestinationDir(args.DestinationDir); err != nil {
		return "", errors.Capture(err)
	}
	for _, entry := range args.DumpEntries {
		if err := checkDumpEntryName(entry.Name); err != nil {
			return "", errors.Capture(err)
		}
	}

	stagingDir, err := os.MkdirTemp(args.DestinationDir, tempPrefix)
	if err != nil {
		return "", errors.Errorf("while making backups staging directory: %w", err)
	}
	// The staging directory is removed on success and on failure.
	defer func() { _ = os.RemoveAll(stagingDir) }()

	archivePaths := NewNonCanonicalArchivePaths(stagingDir)

	// We go with user-only permissions on principle; the directories
	// are short-lived so in practice it shouldn't matter much.
	if err := os.MkdirAll(archivePaths.DBDumpDir, 0700); err != nil {
		return "", errors.Errorf("while creating temp directories: %w", err)
	}

	// The metadata file does not contain the ID or the "finished"
	// data. However, that information is not as critical. The
	// alternatives are either adding the metadata file to the archive
	// after the fact or adding placeholders here for the finished data
	// and filling them in afterward. Neither is particularly trivial.
	metadataReader, err := meta.AsJSONBuffer()
	if err != nil {
		return "", errors.Errorf("while preparing the metadata: %w", err)
	}
	if err := writeAll(archivePaths.MetadataFile, metadataReader); err != nil {
		return "", errors.Capture(err)
	}

	if err := buildFilesBundle(archivePaths.FilesBundle, args.FilesToBackUp); err != nil {
		return "", errors.Capture(err)
	}

	if err := buildDump(archivePaths.DBDumpDir, args.DumpEntries); err != nil {
		return "", errors.Capture(err)
	}

	filename := filepath.Join(args.DestinationDir,
		meta.Started.Format(FilenameTemplate))
	size, checksum, err := buildArchiveAndChecksum(filename, stagingDir,
		archivePaths.ContentDir)
	if err != nil {
		return "", errors.Capture(err)
	}

	if err := meta.MarkComplete(size, checksum); err != nil {
		return "", errors.Errorf("while updating metadata: %w", err)
	}

	return filename, nil
}

// checkDestinationDir ensures the backup destination directory is
// usable.
func checkDestinationDir(destinationDir string) error {
	if !filepath.IsAbs(destinationDir) {
		return errors.Errorf(
			"cannot use relative backup destination directory %q",
			destinationDir)
	}
	if _, err := os.Stat(destinationDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errors.Errorf(
				"backup destination directory %q does not exist",
				destinationDir)
		}
		return errors.Errorf("invalid backup destination directory %q: %w",
			destinationDir, err)
	}
	return nil
}

// checkDumpEntryName ensures the entry name stays within the archive's
// dump directory.
func checkDumpEntryName(name string) error {
	cleaned := path.Clean(name)
	if slices.Contains(strings.Split(cleaned, "/"), "..") {
		return errors.Errorf(
			"dump entry name %q escapes the dump directory: %w",
			name, coreerrors.NotValid)
	}
	return nil
}

// writeAll writes the contents of source to the named file, creating
// any missing parent directories.
func writeAll(targetname string, source io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(targetname), 0700); err != nil {
		return errors.Errorf("while creating directory for %q: %w",
			targetname, err)
	}
	target, err := os.Create(targetname)
	if err != nil {
		return errors.Errorf("while creating file %q: %w", targetname, err)
	}
	if _, err := io.Copy(target, source); err != nil {
		_ = target.Close()
		return errors.Errorf("while copying into file %q: %w", targetname, err)
	}
	if err := target.Close(); err != nil {
		return errors.Errorf("while closing file %q: %w", targetname, err)
	}
	return nil
}

// buildFilesBundle creates the tar file bundling all the juju
// state-related files gathered in by the backup machinery.
func buildFilesBundle(bundleFileName string, filesToBackUp []string) error {
	if len(filesToBackUp) == 0 {
		return errors.New("missing list of files to back up")
	}

	bundleFile, err := os.Create(bundleFileName)
	if err != nil {
		return errors.Errorf("while creating bundle file: %w", err)
	}

	// The leading path separator is stripped off each file name when
	// it is added to the tar file.
	stripPrefix := string(os.PathSeparator)
	_, terr := tar.TarFiles(filesToBackUp, bundleFile, stripPrefix)
	if cerr := bundleFile.Close(); terr == nil {
		terr = errors.Capture(cerr)
	}
	if terr != nil {
		return errors.Errorf("while bundling state-critical files: %w", terr)
	}
	return nil
}

// buildDump writes the database dump entries into the archive's dump
// directory.
func buildDump(dumpDir string, entries []DumpEntry) error {
	for _, entry := range entries {
		target := filepath.Join(dumpDir, filepath.FromSlash(entry.Name))
		if err := writeAll(target, entry.Reader); err != nil {
			return errors.Capture(err)
		}
	}
	return nil
}

// buildArchiveAndChecksum tars and gzips the content directory into
// the named archive file, computing the archive's SHA-1 checksum and
// size along the way.
func buildArchiveAndChecksum(filename, stagingDir, contentDir string) (int64, string, error) {
	archiveFile, err := os.Create(filename)
	if err != nil {
		return 0, "", errors.Errorf("while creating archive file: %w", err)
	}

	// Build the tarball, writing out to both the archive file and a
	// SHA-1 hash. The hash corresponds to the gzipped file rather than
	// to the uncompressed contents of the tarball. This is so that
	// users can compare the published checksum against the checksum of
	// the file without having to decompress it first.
	hasher := hash.NewHashingWriter(archiveFile, sha1.New())
	err = buildArchive(hasher, stagingDir, contentDir)
	if cerr := archiveFile.Close(); err == nil {
		err = errors.Capture(cerr)
	}
	if err != nil {
		return 0, "", errors.Capture(err)
	}

	stat, err := os.Stat(filename)
	if err != nil {
		return 0, "", errors.Errorf("while reading archive file info: %w", err)
	}

	return stat.Size(), hasher.Base64Sum(), nil
}

// buildArchive writes the gzipped tar of the content directory to the
// output.
func buildArchive(outFile io.Writer, stagingDir, contentDir string) error {
	tarball := gzip.NewWriter(outFile)

	// We add a trailing slash (or whatever) to root so that everything
	// in the path up to and including that slash is stripped off when
	// each file is added to the tar file.
	stripPrefix := stagingDir + string(os.PathSeparator)
	filenames := []string{contentDir}
	if _, err := tar.TarFiles(filenames, tarball, stripPrefix); err != nil {
		_ = tarball.Close()
		return errors.Errorf("while bundling final archive: %w", err)
	}

	// Gzip writers may buffer what they're writing so the writer must
	// be closed before the caller reads the checksum from the hasher.
	if err := tarball.Close(); err != nil {
		return errors.Errorf("while closing final archive: %w", err)
	}
	return nil
}

// CheckSpaceFor errors when the free space in dir is less than the
// expected archive size plus a safety margin. The margin is the larger
// of the smaller of 5GiB or 10% of the total disk size, and 20% of the
// expected size.
func CheckSpaceFor(dir string, expectedSize int64) error {
	const (
		miByte          = uint64(1) << 20
		minFreeAbsolute = float64(uint64(5) << 30)
	)

	total, err := diskTotal(dir)
	if err != nil {
		return errors.Capture(err)
	}
	diskSizeMargin := float64(total) * 0.10
	if diskSizeMargin > minFreeAbsolute {
		diskSizeMargin = minFreeAbsolute
	}
	backupSizeMargin := float64(expectedSize) * 0.20
	if backupSizeMargin < diskSizeMargin {
		backupSizeMargin = diskSizeMargin
	}
	wantFree := uint64(expectedSize) + uint64(backupSizeMargin)

	available, err := diskFree(dir)
	if err != nil {
		return errors.Capture(err)
	}
	if available < wantFree {
		return errors.Errorf("not enough free space in %q; want %dMiB, have %dMiB",
			dir, wantFree/miByte, available/miByte)
	}
	return nil
}
