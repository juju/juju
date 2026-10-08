// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	archivetar "archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/juju/clock"
	"github.com/juju/utils/v4/hash"
	utilstar "github.com/juju/utils/v4/tar"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

// tempPrefix is the prefix used for the backup staging directories.
const tempPrefix = "juju-backup-"

// DumpEntry is a single database dump file to include in the backup
// archive, under the archive's dump directory.
type DumpEntry struct {
	// Name is the path of the entry relative to the archive's dump
	// directory, e.g. "controller.yaml" or "models/<uuid>.yaml".
	Name string

	// Reader provides the contents of the dump entry.
	Reader io.Reader
}

// ObjectSource opens the content of one object store object for
// reading. The returned reader must be closed by the caller.
type ObjectSource func(ctx context.Context) (io.ReadCloser, error)

// ObjectEntry is a single object store object to include in the
// backup's root.tar archive.
type ObjectEntry struct {
	// Namespace is the object store namespace the object belongs to:
	// "controller" for the controller database, or a model UUID.
	Namespace string

	// SHA256 is the hex-encoded SHA-256 of the object, as exported by
	// the database. It identifies the object for reading.
	SHA256 string

	// SHA384 is the hex-encoded SHA-384 of the object, as exported by
	// the database. It names the object inside the archive.
	SHA384 string

	// Size is the expected object size in bytes.
	Size int64

	// Source opens the object for reading.
	Source ObjectSource
}

// CreateArgs holds the arguments for building a backup archive.
type CreateArgs struct {
	// DestinationDir is the absolute path to the directory in which
	// the archive is stored. The staging area is created there too.
	DestinationDir string

	// DataDir is the absolute path to the source controller's data
	// directory; object entries are stored relative to it. Required
	// when ObjectEntries is not empty.
	DataDir string

	// FilesToBackUp is the list of absolute paths to the files to
	// include in the archive's root.tar.
	FilesToBackUp []string

	// ObjectEntries are the object store objects archived into the
	// archive's root.tar, streamed and validated one at a time.
	ObjectEntries []ObjectEntry

	// DumpEntries are the database dumps written under the archive's
	// dump directory.
	DumpEntries []DumpEntry

	// Clock provides the time stamping the metadata when the archive
	// creation finishes. It must not be nil.
	Clock clock.Clock
}

// Create builds a new backup archive file in args.DestinationDir,
// named after meta.Started using FilenameTemplate. It updates the
// metadata with the file info and returns the archive filename.
// It is a variable so tests can replace the archive creation with a
// stub, mirroring [GetFilesToBackUp].
var Create = func(ctx context.Context, meta *Metadata, args CreateArgs) (string, error) {
	if args.Clock == nil {
		return "", errors.New("missing clock")
	}
	if err := checkDestinationDir(args.DestinationDir); err != nil {
		return "", errors.Capture(err)
	}
	for _, entry := range args.DumpEntries {
		if err := checkDumpEntryName(entry.Name); err != nil {
			return "", errors.Capture(err)
		}
	}
	if err := checkObjectEntries(args.ObjectEntries, args.DataDir); err != nil {
		return "", errors.Capture(err)
	}

	stagingDir, err := os.MkdirTemp(args.DestinationDir, tempPrefix)
	if err != nil {
		return "", errors.Errorf("making backups staging directory: %w", err)
	}
	// The staging directory is removed on success and on failure.
	defer func() { _ = os.RemoveAll(stagingDir) }()

	archivePaths := NewNonCanonicalArchivePaths(stagingDir)

	// We go with user-only permissions on principle; the directories
	// are short-lived so in practice it shouldn't matter much.
	if err := os.MkdirAll(archivePaths.DBDumpDir, 0700); err != nil {
		return "", errors.Errorf("creating temp directories: %w", err)
	}

	// The metadata file does not contain the ID or the "finished"
	// data. However, that information is not as critical. The
	// alternatives are either adding the metadata file to the archive
	// after the fact or adding placeholders here for the finished data
	// and filling them in afterward. Neither is particularly trivial.
	metadataReader, err := meta.AsJSONBuffer()
	if err != nil {
		return "", errors.Errorf("preparing the metadata: %w", err)
	}
	if err := writeAll(archivePaths.MetadataFile, metadataReader); err != nil {
		return "", errors.Capture(err)
	}

	if err := buildFilesArchive(ctx, archivePaths.FilesArchive, args.FilesToBackUp, args.DataDir, args.ObjectEntries); err != nil {
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
	// The archive now exists under its final name. Every failure from
	// here on must discard it: buildArchiveAndChecksum cleans up after
	// its own failures, but a stray archive left behind here is one the
	// caller was told was never created, and it still shows up to list
	// and download. Any step added below has to do the same.
	if err := meta.MarkComplete(size, checksum, args.Clock.Now()); err != nil {
		return "", discardArchive(filename,
			errors.Errorf("updating metadata: %w", err))
	}

	return filename, nil
}

// discardArchive removes a written archive that its caller is about to
// report as failed, and returns the failure to report. A removal error
// is folded into it rather than swallowed, since it leaves exactly the
// stray archive the removal was there to prevent.
func discardArchive(filename string, err error) error {
	if rerr := os.Remove(filename); rerr != nil && !os.IsNotExist(rerr) {
		return errors.Errorf("%w (also removing %q: %v)", err, filename, rerr)
	}
	return err
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

// checkDumpEntryName ensures the entry name is not empty, is relative
// and stays within the archive's dump directory.
func checkDumpEntryName(name string) error {
	cleaned := path.Clean(name)
	// path.Clean maps both "" and "." (and "./", ...) to ".", which
	// would resolve to the dump directory itself.
	if cleaned == "." {
		return errors.Errorf("empty dump entry name %q: %w",
			name, coreerrors.NotValid)
	}
	if path.IsAbs(cleaned) ||
		slices.Contains(strings.Split(cleaned, "/"), "..") {
		return errors.Errorf(
			"entry name %q escapes the root directory: %w",
			name, coreerrors.NotValid)
	}
	return nil
}

// writeAll writes the contents of source to the named file, creating
// any missing parent directories.
func writeAll(targetname string, source io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(targetname), 0700); err != nil {
		return errors.Errorf("creating directory for %q: %w",
			targetname, err)
	}
	target, err := os.Create(targetname)
	if err != nil {
		return errors.Errorf("creating file %q: %w", targetname, err)
	}
	if _, err := io.Copy(target, source); err != nil {
		_ = target.Close()
		return errors.Errorf("copying into file %q: %w", targetname, err)
	}
	if err := target.Close(); err != nil {
		return errors.Errorf("closing file %q: %w", targetname, err)
	}
	return nil
}

// buildFilesArchive creates the tar file archiving all the juju
// state-related files gathered in by the backup machinery, followed by
// the object store objects referenced by the database dumps.
func buildFilesArchive(ctx context.Context, archiveFileName string, filesToBackUp []string, dataDir string, objectEntries []ObjectEntry) error {
	if len(filesToBackUp) == 0 {
		return errors.New("missing list of files to back up")
	}

	// Create the parent directory here rather than relying on an
	// earlier write having created it, matching writeAll.
	if err := os.MkdirAll(filepath.Dir(archiveFileName), 0700); err != nil {
		return errors.Errorf("creating directory for %q: %w",
			archiveFileName, err)
	}

	archiveFile, err := os.Create(archiveFileName)
	if err != nil {
		return errors.Errorf("creating archive file: %w", err)
	}

	// Both the filesystem files and the object entries go into a
	// single tar stream, so the archive is written by one tar.Writer.
	tarw := archivetar.NewWriter(archiveFile)
	fail := func(err error) error {
		if terr := tarw.Close(); err == nil {
			err = errors.Capture(terr)
		}
		if cerr := archiveFile.Close(); err == nil {
			err = errors.Capture(cerr)
		}
		return err
	}
	// The leading path separator is stripped off each file name when
	// it is added to the tar file.
	stripPrefix := string(os.PathSeparator)
	for _, file := range filesToBackUp {
		if err := writeArchiveFile(tarw, file, stripPrefix); err != nil {
			return fail(errors.Errorf("archiving state-critical file %q: %w", file, err))
		}
	}
	for _, entry := range objectEntries {
		if err := writeObjectEntry(ctx, tarw, dataDir, entry); err != nil {
			return fail(errors.Capture(err))
		}
	}

	if err := tarw.Close(); err != nil {
		return fail(errors.Capture(err))
	}
	if err := archiveFile.Close(); err != nil {
		return errors.Errorf("closing files archive: %w", err)
	}
	return nil
}

// writeArchiveFile adds a single file or directory to the archive, with the
// strip prefix removed from its stored name. Symlinks retain their link
// targets without being followed, directories are stored as headers and
// regular files are copied in full.
func writeArchiveFile(tarw *archivetar.Writer, fileName, stripPrefix string) error {
	fInfo, err := os.Lstat(fileName)
	if err != nil {
		return errors.Capture(err)
	}
	link := ""
	if fInfo.Mode()&os.ModeSymlink != 0 {
		link, err = os.Readlink(fileName)
		if err != nil {
			return errors.Capture(err)
		}
	}
	hdr, err := archivetar.FileInfoHeader(fInfo, link)
	if err != nil {
		return errors.Capture(err)
	}
	hdr.Name = filepath.ToSlash(strings.TrimPrefix(fileName, stripPrefix))
	if err := tarw.WriteHeader(hdr); err != nil {
		return errors.Capture(err)
	}
	if fInfo.Mode().IsRegular() {
		f, err := os.Open(fileName)
		if err != nil {
			return errors.Capture(err)
		}
		defer func() { _ = f.Close() }()

		if _, err := io.CopyN(tarw, f, fInfo.Size()); err != nil {
			return errors.Capture(err)
		}
	}
	return nil
}

// writeObjectEntry streams one object store object into the archive,
// validating the byte count and SHA-384 hash while copying. The caller
// context is checked before every object so a cancelled backup does not
// keep streaming.
func writeObjectEntry(ctx context.Context, tarw *archivetar.Writer, dataDir string, entry ObjectEntry) error {
	if err := ctx.Err(); err != nil {
		return errors.Capture(err)
	}
	reader, err := entry.Source(ctx)
	if err != nil {
		return errors.Errorf("opening object %q in namespace %q: %w",
			entry.SHA384, entry.Namespace, err)
	}
	defer func() { _ = reader.Close() }()

	hdr := &archivetar.Header{
		Name:     objectEntryArchivePath(dataDir, entry),
		Mode:     0640,
		Size:     entry.Size,
		Typeflag: archivetar.TypeReg,
	}
	if err := tarw.WriteHeader(hdr); err != nil {
		return errors.Errorf("writing object header for %q: %w", hdr.Name, err)
	}

	hash384 := sha512.New384()
	tee := io.TeeReader(reader, hash384)
	// CopyN reports a short stream as io.EOF and a long one leaves the
	// trailing bytes unread, so both surface as size mismatches.
	n, err := io.CopyN(tarw, tee, entry.Size)
	if err != nil && !errors.Is(err, io.EOF) {
		return errors.Errorf("writing object %q in namespace %q: %w",
			entry.SHA384, entry.Namespace, err)
	}
	if n != entry.Size {
		return errors.Errorf("object %q in namespace %q: streamed %d bytes, expected %d",
			entry.SHA384, entry.Namespace, n, entry.Size)
	}
	if sum := hex.EncodeToString(hash384.Sum(nil)); sum != entry.SHA384 {
		return errors.Errorf("object %q in namespace %q: SHA-384 mismatch: got %q, expected %q",
			entry.SHA384, entry.Namespace, sum, entry.SHA384)
	}
	return nil
}

// objectEntryArchivePath returns the path of the entry's object within the
// files archive, mirroring the object store's on-disk layout relative to the
// data directory so restore can place it without translation.
func objectEntryArchivePath(dataDir string, entry ObjectEntry) string {
	return path.Join(
		strings.TrimPrefix(filepath.ToSlash(dataDir), "/"),
		objectstoreDir, entry.Namespace, entry.SHA384,
	)
}

// checkObjectEntries validates the object entries before the archive is
// built. A duplicated (namespace, SHA-384) pair would produce a duplicate
// path in the archive, which is a verification failure point on restore.
func checkObjectEntries(entries []ObjectEntry, dataDir string) error {
	if len(entries) == 0 {
		return nil
	}
	if dataDir == "" {
		return errors.New("missing data directory for object entries")
	}
	type key struct {
		namespace string
		sha384    string
	}
	seen := make(map[key]struct{}, len(entries))
	for _, entry := range entries {
		if entry.Namespace == "" || entry.SHA384 == "" || entry.Source == nil {
			return errors.Errorf("incomplete object entry: %w", coreerrors.NotValid)
		}
		if entry.Size < 0 {
			return errors.Errorf("object %q in namespace %q: negative size %d: %w",
				entry.SHA384, entry.Namespace, entry.Size, coreerrors.NotValid)
		}
		k := key{namespace: entry.Namespace, sha384: entry.SHA384}
		if _, dup := seen[k]; dup {
			return errors.Errorf("duplicate object entry %q in namespace %q: %w",
				entry.SHA384, entry.Namespace, coreerrors.NotValid)
		}
		seen[k] = struct{}{}
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
func buildArchiveAndChecksum(filename, stagingDir, contentDir string) (_ int64, _ string, err error) {
	archiveFile, err := os.Create(filename)
	if err != nil {
		return 0, "", errors.Errorf("creating archive file: %w", err)
	}
	// The archive is only complete once its final flush lands, so a
	// close failure fails the backup unless the build already failed.
	// A failed build leaves no partial archive behind.
	defer func() {
		if cerr := archiveFile.Close(); err == nil && cerr != nil {
			err = errors.Errorf("closing archive file: %w", cerr)
		}
		if err != nil {
			_ = os.Remove(filename)
		}
	}()

	// Build the tarball, writing out to both the archive file and a
	// SHA-1 hash. The hash corresponds to the gzipped file rather than
	// to the uncompressed contents of the tarball. This is so that
	// users can compare the published checksum against the checksum of
	// the file without having to decompress it first.
	hasher := hash.NewHashingWriter(archiveFile, sha1.New())
	if err := buildArchive(hasher, stagingDir, contentDir); err != nil {
		return 0, "", errors.Capture(err)
	}

	stat, err := os.Stat(filename)
	if err != nil {
		return 0, "", errors.Errorf("reading archive file info: %w", err)
	}

	return stat.Size(), hasher.Base64Sum(), nil
}

// buildArchive writes the gzipped tar of the content directory to the
// output.
func buildArchive(outFile io.Writer, stagingDir, contentDir string) error {
	tarball := gzip.NewWriter(outFile)

	// A trailing path separator is appended to the staging directory
	// so that everything in the path up to and including that
	// separator is stripped off when each file is added to the tar
	// file.
	stripPrefix := stagingDir + string(os.PathSeparator)
	filenames := []string{contentDir}
	if _, err := utilstar.TarFiles(filenames, tarball, stripPrefix); err != nil {
		_ = tarball.Close()
		return errors.Errorf("creating final archive: %w", err)
	}

	// Gzip writers may buffer what they're writing so the writer must
	// be closed before the caller reads the checksum from the hasher.
	if err := tarball.Close(); err != nil {
		return errors.Errorf("closing final archive: %w", err)
	}
	return nil
}

const (
	// miByte is one mebibyte (2^20 bytes); free-space shortfalls are
	// reported in MiB.
	miByte = uint64(1) << 20
	// minFreeAbsolute caps the disk-size margin at 5 GiB
	// (5 * 2^30 bytes).
	minFreeAbsolute = float64(uint64(5) << 30)
)

// CheckSpaceFor errors when the free space in dir is less than the
// expected archive size plus a safety margin. The margin is the larger
// of the smaller of 5GiB or 10% of the total disk size, and 20% of the
// expected size. It is a variable so tests can trigger the
// insufficient-space failure, mirroring [Create].
var CheckSpaceFor = func(dir string, expectedSize int64) error {
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
