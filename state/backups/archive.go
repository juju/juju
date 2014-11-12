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

// ArchivePaths represents the contents of a backup archive.
// The archive may be one packed into an archive file, unpacked into
// some directory on the filesystem, or both.  Regardless, the contents
// remain the same.
type ArchivePaths struct {
	// RootDir is the path relative to which the various paths to
	// archive content are resolve.  For a tar file it will be empty.
	// For the unpacked content, it is the directory into which the
	// backup arechive was unpacked.
	RootDir string
	// UseCanonicalPaths indicates that the generated paths should be
	// joined by "/".  Otherwise the path separators will be platform-
	// dependent.
	UseCanonicalPaths bool
}

// NewUnpackedArchivePaths prepares backup archive paths relative to
// the provided root directory. The path separators of the relative
// paths will be platform-specific.
func NewUnpackedArchivePaths(rootdir string) ArchivePaths {
	return ArchivePaths{rootdir, false}
}

// NewCanonicalArchivePaths prepares backup archive paths with no
// relative root directory. The canonical slash ("/") will be used for
// the path separators.
func NewCanonicalArchivePaths(rootdir string) ArchivePaths {
	rootdir = filepath.ToSlash(rootdir)
	return ArchivePaths{rootdir, true}
}

// NewUnpackedArchivePaths prepares backup archive paths with no
// relative root directory. The canonical slash ("/") will be used for
// the path separators.
func NewPackedArchivePaths() ArchivePaths {
	return NewCanonicalArchivePaths("")
}

// ContentDir is the path to the directory within the archive containing
// all the contents.  It is the only file or directory at the top-level
// of the archive and everything else in the archive is contained in the
// content directory.
func (ar ArchivePaths) ContentDir() string {
	if ar.UseCanonicalPaths {
		return path.Join(ar.RootDir, contentDir)
	} else {
		return filepath.Join(ar.RootDir, contentDir)
	}
}

// FilesBundle is the path to the tar file inside the archive containing
// all the state-related files (with the exception of the DB dump files)
// gathered in by the backup machinery.
func (ar ArchivePaths) FilesBundle() string {
	if ar.UseCanonicalPaths {
		return path.Join(ar.ContentDir(), filesBundle)
	} else {
		return filepath.Join(ar.ContentDir(), filesBundle)
	}
}

// DBDumpDir is the path to the directory within the archive contents
// that contains all the files dumped from the juju state database.
func (ar ArchivePaths) DBDumpDir() string {
	if ar.UseCanonicalPaths {
		return path.Join(ar.ContentDir(), dbDumpDir)
	} else {
		return filepath.Join(ar.ContentDir(), dbDumpDir)
	}
}

// MetadataFile is the path to the metadata file.
func (ar ArchivePaths) MetadataFile() string {
	if ar.UseCanonicalPaths {
		return path.Join(ar.ContentDir(), metadataFile)
	} else {
		return filepath.Join(ar.ContentDir(), metadataFile)
	}
}
