// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

// TODO(ericsnow) Move this file elsewhere?
//  (e.g. top-level resource pkg, charm/resource)

import (
	"io"

	"github.com/juju/errors"
)

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

type DownloadTarget interface {
	Open() (DownloadDirectory, error)
}

type DownloadDirectory interface {
	Write(ContentSource) error
}

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

type DownloadIndirectDeps interface {
	// NewTempDirSpec returns a directory spec for the resource under a temporary datadir.
	NewTempDirSpec() (DownloadTempTarget, error)

	// CloseAndLog closes the closer and logs any error.
	CloseAndLog(io.Closer, string)

	// ReplaceDirectory moves the source directory path to the target
	// path. If the target path already exists then it is replaced atomically.
	ReplaceDirectory(tgt, src string) error
}

type DownloadTempTarget interface {
	DownloadTarget
	Resolver
	io.Closer
}
