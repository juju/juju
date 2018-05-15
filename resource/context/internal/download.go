// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

// TODO(ericsnow) Move this file elsewhere?
//  (e.g. top-level resource pkg, charm/resource)

import (
	"github.com/juju/errors"
)

// Download downloads the resource from the provied source to the target.
func Download(target DownloadTarget, remote ContentSource) error {
	resDir, err := target.Initialize()
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
	// Initialize prepares the target directory and returns it.
	Initialize() (DownloadDirectory, error)
}

// DownloadDirectory exposes the functionality of a resource directory
// needed by Download().
type DownloadDirectory interface {
	Resolver

	// Write writes all the relevant files for the provided source
	// to the directory.
	Write(ContentSource) error
}

// Resolver exposes the functionality of DirectorySpec needed
// by DownloadIndirect.
type Resolver interface {
	// Resolve returns the fully resolved path for the provided path items.
	Resolve(...string) string
}
