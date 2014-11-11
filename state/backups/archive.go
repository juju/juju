// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"path/filepath"
)

const (
	contentDir   = "juju-backup"
	filesBundle  = "root.tar"
	dbDumpDir    = "dump"
	metadataFile = "metadata.json"
)

// Archive is used to represent the contents of a backup archive.  This
// archive may be packed into an archive file, unpacked into some
// directory on the filesystem, or both.  Regardless, the contents
// remain the same.
type Archive struct {
	// RootDir is the path relative to which the various paths to
	// archive content are resolve.  For a tar file it will be empty.
	// For the unpacked content, it is the directory into which the
	// backup arechive was unpacked.
	RootDir string
}

// ContentDir is the path to the directory within the archive containing
// all the contents.  It is the only file or directory at the top-level
// of the archive and everything else in the archive is contained in the
// content directory.
func (ar Archive) ContentDir() string {
	return filepath.Join(ar.RootDir, contentDir)
}

// FilesBundle is the path to the tar file inside the archive containing
// all the state-related files (with the exception of the DB dump files)
// gathered in by the backup machinery.
func (ar Archive) FilesBundle() string {
	return filepath.Join(ar.ContentDir(), filesBundle)
}

// DBDumpDir is the path to the directory within the archive contents
// that contains all the files dumped from the juju state database.
func (ar Archive) DBDumpDir() string {
	return filepath.Join(ar.ContentDir(), dbDumpDir)
}

// MetadataFile is the path to the metadata file.
func (ar Archive) MetadataFile() string {
	return filepath.Join(ar.ContentDir(), metadataFile)
}
