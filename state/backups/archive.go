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

// ArchivePaths represents the contents of a backup archive. The archive
// may be one packed into an archive file, unpacked into some directory
// on the filesystem, or both. Regardless, the contents remain the same.
type ArchivePaths struct {
	// RootDir is the path relative to which the various paths to
	// archive content resolve. For a tar file it will be empty.
	// For the unpacked content, it is the directory into which the
	// backup arechive was unpacked.
	RootDir string

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

// NewArchivePaths prepares backup archive paths relative to the
// specified root directory. If canonical is true, the paths (including
// RootDir) will use slash ("/") as the path separator. Otherwise they
// will be platform-specific.
func NewArchivePaths(rootdir string, canonical bool) ArchivePaths {
	if canonical {
		rootdir = filepath.ToSlash(rootdir)
		return ArchivePaths{
			RootDir:      rootdir,
			ContentDir:   path.Join(rootdir, contentDir),
			FilesBundle:  path.Join(rootdir, contentDir, filesBundle),
			DBDumpDir:    path.Join(rootdir, contentDir, dbDumpDir),
			MetadataFile: path.Join(rootdir, contentDir, metadataFile),
		}
	} else {
		return ArchivePaths{
			RootDir:      rootdir,
			ContentDir:   filepath.Join(rootdir, contentDir),
			FilesBundle:  filepath.Join(rootdir, contentDir, filesBundle),
			DBDumpDir:    filepath.Join(rootdir, contentDir, dbDumpDir),
			MetadataFile: filepath.Join(rootdir, contentDir, metadataFile),
		}
	}
}

// NewUnpackedArchivePaths prepares backup archive paths relative to
// the provided root directory. The path separators of the relative
// paths will be platform-specific.
func NewUnpackedArchivePaths(rootdir string) ArchivePaths {
	return NewArchivePaths(rootdir, false)
}

// NewPackedArchivePaths prepares backup archive paths with no
// relative root directory. Slash ("/") will be used for the path
// separators.
func NewPackedArchivePaths() ArchivePaths {
	return NewArchivePaths("", true)
}
