// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package archive

import (
	"path/filepath"
)

const (
	contentDir  = "juju-backup"
	filesBundle = "root.tar"
	dbDumpDir   = "dump"
)

type Archive struct {
	filename string
	rootDir  string
}

// NewRootedArchive returns a new archive summary for the filename and
// root directory for the unpacked contents.
func NewRootedArchive(filename, rootDir string) *Archive {
	ar := Archive{
		filename: filename,
		rootDir:  rootDir,
	}
	return &ar
}

// NewArchive returns a new archive summary for the filename.
func NewArchive(filename string) *Archive {
	ar := Archive{
		filename: filename,
	}
	return &ar
}

// Filename is the path to the archive file.
func (ar *Archive) Filename() string {
	return ar.filename
}

// RootDir is the path to the root directory for the unpacked contents.
func (ar *Archive) RootDir() string {
	return ar.rootDir
}

// ContentDir is the path to the directory within the archive containing
// all the contents.
func (ar *Archive) ContentDir() string {
	return filepath.Join(ar.rootDir, contentDir)
}

// FilesBundle is the path to the tar file inside the archive containing
// all the state-related files gathered in by the backup machinery.
func (ar *Archive) FilesBundle() string {
	return filepath.Join(ar.ContentDir(), filesBundle)
}

// DBDumpDir is the path to the directory within the archive contents
// that contains all the files dumped from the juju state database.
func (ar *Archive) DBDumpDir() string {
	return filepath.Join(ar.ContentDir(), dbDumpDir)
}
