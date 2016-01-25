// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

// TODO(ericsnow) Move this file elsewhere?
//  (e.g. top-level resource pkg, charm/resource)

import (
	"io"

	"github.com/juju/errors"
)

// Download downloads the resource from the provied source to the target.
func Download(target DownloadTarget, remote ContentSource) error {
	resDir, err := target.Open()
	if err != nil {
		return errors.Trace(err)
	}

	if err := resDir.Write(remote); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// DownloadTarget exposes the functionality of a directory spec
// needed by Download().
type DownloadTarget interface {
	// Open prepares the target directory and returns it.
	Open() (DownloadDirectory, error)
}

// DownloadDirectory exposes the functionality of a resource directory
// needed by Download().
type DownloadDirectory interface {
	// Write writes all the relevant files for the provided source
	// to the directory.
	Write(ContentSource) error
}

// DownloadIndirect downloads the resource from the source into a temp
// directory. Then the target is replaced by the temp directory.
func DownloadIndirect(target Resolver, remote ContentSource, deps DownloadIndirectDeps) error {
	tempDirSpec, err := deps.NewTempDirSpec()
	defer deps.CloseAndLog(tempDirSpec, "resource temp dir")
	if err != nil {
		return errors.Trace(err)
	}

	if err := Download(tempDirSpec, remote); err != nil {
		return errors.Trace(err)
	}

	oldDir := tempDirSpec.Resolve()
	newDir := target.Resolve()
	if err := deps.ReplaceDirectory(newDir, oldDir); err != nil {
		return errors.Annotate(err, "could not replace existing resource directory")
	}

	return nil
}

// Resolver exposes the functionality of DirectorySpec needed
// by DownloadIndirect.
type Resolver interface {
	// Resolve returns the fully resolved path for the provided path items.
	Resolve(...string) string
}

// DownloadIndirectDeps exposes the external functionality needed
// by DownloadIndirect.
type DownloadIndirectDeps interface {
	// NewTempDirSpec returns a directory spec for the resource under a temporary datadir.
	NewTempDirSpec() (DownloadTempTarget, error)

	// CloseAndLog closes the closer and logs any error.
	CloseAndLog(io.Closer, string)

	// ReplaceDirectory moves the source directory path to the target
	// path. If the target path already exists then it is replaced atomically.
	ReplaceDirectory(tgt, src string) error
}

// DownloadTempTarget represents a temporary download directory.
type DownloadTempTarget interface {
	DownloadTarget
	Resolver
	io.Closer
}
