// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"io"

	"github.com/juju/errors"
)

// ContextDownload downloads the named resource and returns the path
// to which it was downloaded. If the resource does not exist or has
// not been uploaded yet then errors.NotFound is returned.
//
// Note that the downloaded file is checked for correctness.
func ContextDownload(deps ContextDownloadDeps) (path string, err error) {
	// TODO(katco): Potential race-condition: two commands running at
	// once. Solve via collision using os.Mkdir() with a uniform
	// temp dir name (e.g. "<datadir>/.<res name>.download")?

	resDirSpec := deps.NewContextDirectorySpec()

	remote, err := deps.OpenResource()
	if err != nil {
		return "", errors.Trace(err)
	}
	defer deps.CloseAndLog(remote, "remote resource")
	path = resDirSpec.Resolve(remote.Info().Path)

	isUpToDate, err := resDirSpec.IsUpToDate(remote.Content())
	if err != nil {
		return "", errors.Trace(err)
	}
	if isUpToDate {
		// We're up to date already!
		return path, nil
	}

	if err := deps.Download(resDirSpec, remote); err != nil {
		return "", errors.Trace(err)
	}

	return path, nil
}

// ContextDownloadDeps provides the externally defined functions
// on which ContextDownload depends. The functionality all relates
// to a single resource.
type ContextDownloadDeps interface {
	// NewContextDirectorySpec returns the dir spec for the resource
	// in the hook context.
	NewContextDirectorySpec() ContextDirectorySpec

	// OpenResource reads the resource info and opens the resource
	// content for reading.
	OpenResource() (ContextOpenedResource, error)

	// CloseAndLog closes the closer and logs any error.
	CloseAndLog(io.Closer, string)

	// Download writes the remote to the target directory.
	Download(DownloadTarget, ContextOpenedResource) error
}

// ContextDirectorySpec exposes the functionality of a resource dir spec
// in a hook context.
type ContextDirectorySpec interface {
	Resolver

	// Initializeprepares the target directory and returns it.
	Initialize() (DownloadDirectory, error)

	// IsUpToDate indicates whether or not the resource dir is in sync
	// with the content.
	IsUpToDate(Content) (bool, error)
}

// NewContextDirectorySpec returns a new directory spec for the context.
func NewContextDirectorySpec(dataDir, name string, deps DirectorySpecDeps) ContextDirectorySpec {
	return &contextDirectorySpec{
		DirectorySpec: NewDirectorySpec(dataDir, name, deps),
	}
}

type contextDirectorySpec struct {
	*DirectorySpec
}

// Initializeimplements ContextDirectorySpec.
func (spec contextDirectorySpec) Initialize() (DownloadDirectory, error) {
	return spec.DirectorySpec.Initialize()
}

// ContextDownloadDirectory is an adapter for TempDirectorySpec.
type ContextDownloadDirectory struct {
	*TempDirectorySpec
}

// Initialize implements DownloadTarget.
func (dir ContextDownloadDirectory) Initialize() (DownloadDirectory, error) {
	return dir.TempDirectorySpec.Initialize()
}

// ContextOpenedResource exposes the functionality of an "opened"
// resource.
type ContextOpenedResource interface {
	ContentSource
	io.Closer
}

// NewContextContentChecker returns a content checker for the hook context.
func NewContextContentChecker(content Content, deps NewContextContentCheckerDeps) ContentChecker {
	if content.Fingerprint.IsZero() {
		return &NopChecker{}
	}

	sizer := deps.NewSizeTracker()
	checksumWriter := deps.NewChecksumWriter()
	//checker.checksumWriter = charmresource.NewFingerprintHash()
	return NewContentChecker(content, sizer, checksumWriter)
}

// NewContextContentCheckerDeps exposes the functionality needed
// by NewContextContentChecker().
type NewContextContentCheckerDeps interface {
	// NewSizeTracker returns a new size tracker.
	NewSizeTracker() SizeTracker

	// NewChecksumWriter returns a new checksum writer.
	NewChecksumWriter() ChecksumWriter
}
