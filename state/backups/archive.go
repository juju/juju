// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"bytes"
	"compress/gzip"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/utils/tar"

	"github.com/juju/juju/version"
)

const (
	contentDir   = "juju-backup"
	filesBundle  = "root.tar"
	dbDumpDir    = "dump"
	metadataFile = "metadata.json"
)

var legacyVersion = version.Number{Major: 1, Minor: 20}

// ArchivePaths holds the paths to the files and directories in a
// backup archive.
type ArchivePaths struct {
	// ContentDir is the path to the directory within the archive
	// containing all the contents. It is the only file or directory at
	// the top-level of the archive and everything else in the archive
	// is contained in the content directory.
	ContentDir string

	// FilesBundle is the path to the tar file inside the archive
	// containing all the state-related files (with the exception of the
	// DB dump files) gathered in by the backup machinery.
	FilesBundle string

	// DBDumpDir is the path to the directory within the archive
	// contents that contains all the files dumped from the juju state
	// database.
	DBDumpDir string

	// MetadataFile is the path to the metadata file.
	MetadataFile string
}

// NewCanonicalArchivePaths composes a new ArchivePaths with default
// values set. These values are relative (un-rooted) and the canonical
// slash ("/") is the path separator. Thus the paths are suitable for
// resolving the paths in a backup archive file (which is a tar file).
func NewCanonicalArchivePaths() ArchivePaths {
	return ArchivePaths{
		ContentDir:   contentDir,
		FilesBundle:  path.Join(contentDir, filesBundle),
		DBDumpDir:    path.Join(contentDir, dbDumpDir),
		MetadataFile: path.Join(contentDir, metadataFile),
	}
}

// NonCanonicalArchivePaths builds a new ArchivePaths using default
// values, rooted at the provided rootDir. The path separator used is
// platform-dependent. The resulting paths are suitable for locating
// backup archive contents in a directory into which an archive has
// been unpacked.
func NewNonCanonicalArchivePaths(rootDir string) ArchivePaths {
	return ArchivePaths{
		ContentDir:   filepath.Join(rootDir, contentDir),
		FilesBundle:  filepath.Join(rootDir, contentDir, filesBundle),
		DBDumpDir:    filepath.Join(rootDir, contentDir, dbDumpDir),
		MetadataFile: filepath.Join(rootDir, contentDir, metadataFile),
	}
}

// ArchiveWorkspace is a wrapper around backup archive info that has a
// concrete root directory and an archive unpacked in it.
type ArchiveWorkspace struct {
	ArchivePaths
	RootDir string
}

func newArchiveWorkspace() (*ArchiveWorkspace, error) {
	rootdir, err := ioutil.TempDir("", "juju-backups-")
	if err != nil {
		return nil, errors.Annotate(err, "while creating workspace dir")
	}

	ws := ArchiveWorkspace{
		ArchivePaths: NewNonCanonicalArchivePaths(rootdir),
		RootDir:      rootdir,
	}
	return &ws, nil
}

// NewArchiveWorkspaceReader returns a new archive workspace with a new
// workspace dir populated from the archive. Note that this involves
// unpacking the entire archive into a directory under the host's
// "temporary" directory. For relatively large archives this could have
// adverse effects on hosts with little disk space.
func NewArchiveWorkspaceReader(archive io.Reader) (*ArchiveWorkspace, error) {
	ws, err := newArchiveWorkspace()
	if err != nil {
		return nil, errors.Trace(err)
	}
	err = unpackCompressedReader(ws.RootDir, archive)
	return ws, errors.Trace(err)
}

func unpackCompressedReader(targetDir string, tarFile io.Reader) error {
	tarFile, err := gzip.NewReader(tarFile)
	if err != nil {
		return errors.Annotate(err, "while uncompressing archive file")
	}
	err = tar.UntarFiles(tarFile, targetDir)
	return errors.Trace(err)
}

// Close cleans up the workspace dir.
func (ws *ArchiveWorkspace) Close() error {
	err := os.RemoveAll(ws.RootDir)
	return errors.Trace(err)
}

// UnpackFilesBundle unpacks the archived files bundle into the targeted dir.
func (ws *ArchiveWorkspace) UnpackFilesBundle(targetRoot string) error {
	tarFile, err := os.Open(ws.FilesBundle)
	if err != nil {
		return errors.Trace(err)
	}
	defer tarFile.Close()

	err = tar.UntarFiles(tarFile, targetRoot)
	return errors.Trace(err)
}

// OpenBundledFile returns an open ReadCloser for the corresponding file in
// the archived files bundle.
func (ws *ArchiveWorkspace) OpenBundledFile(filename string) (io.Reader, error) {
	if filepath.IsAbs(filename) {
		return nil, errors.Errorf("filename must be relative, got %q", filename)
	}

	tarFile, err := os.Open(ws.FilesBundle)
	if err != nil {
		return nil, errors.Trace(err)
	}

	_, file, err := tar.FindFile(tarFile, filename)
	if err != nil {
		tarFile.Close()
		return nil, errors.Trace(err)
	}
	return file, nil
}

// Metadata returns the metadata derived from the JSON file in the archive.
func (ws *ArchiveWorkspace) Metadata() (*Metadata, error) {
	metaFile, err := os.Open(ws.MetadataFile)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer metaFile.Close()

	meta, err := NewMetadataJSONReader(metaFile)
	return meta, errors.Trace(err)
}

// ArchiveData is a wrapper around a the uncompressed data in a backup
// archive file. It provides access to the content of the archive. While
// ArchiveData provides useful functionality, it may not be appropriate
// for large archives. The contents of the archive are kept in-memory,
// so large archives could be too taxing on the host. In that case
// consider using ArchiveWorkspace instead.
type ArchiveData struct {
	ArchivePaths
	data []byte
}

// NewArchiveData builds a new archive data wrapper for the given
// uncompressed data.
func NewArchiveData(data []byte) *ArchiveData {
	return &ArchiveData{
		ArchivePaths: NewCanonicalArchivePaths(),
		data:         data,
	}
}

// NewArchiveReader returns a new archive data wrapper for the data in
// the provided reader. Note that the entire archive will be read into
// memory and kept there. So for relatively large archives it will often
// be more appropriate to use ArchiveWorkspace instead.
func NewArchiveDataReader(r io.Reader) (*ArchiveData, error) {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer gzr.Close()

	data, err := ioutil.ReadAll(gzr)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return NewArchiveData(data), nil
}

// NewBuffer wraps the archive data in a Buffer.
func (ad *ArchiveData) NewBuffer() *bytes.Buffer {
	return bytes.NewBuffer(ad.data)
}

// Metadata returns the metadata stored in the backup archive.  If no
// metadata is there, errors.NotFound is returned.
func (ad *ArchiveData) Metadata() (*Metadata, error) {
	buf := ad.NewBuffer()
	_, metaFile, err := tar.FindFile(buf, ad.MetadataFile)
	if err != nil {
		return nil, errors.Trace(err)
	}

	meta, err := NewMetadataJSONReader(metaFile)
	return meta, errors.Trace(err)
}

// Version returns the juju version under which the backup archive
// was created.  If no version is found in the archive, it must come
// from before backup archives included the version.  In that case we
// return version 1.20.
func (ad *ArchiveData) Version() (*version.Number, error) {
	meta, err := ad.Metadata()
	if errors.IsNotFound(err) {
		return &legacyVersion, nil
	}
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &meta.Origin.Version, nil
}
