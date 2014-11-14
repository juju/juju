// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"path"
	"path/filepath"
)

const (
	contentDir   = "juju-backup"
	filesBundle  = "root.tar"
	dbDumpDir    = "dump"
	metadataFile = "metadata.json"
)

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
