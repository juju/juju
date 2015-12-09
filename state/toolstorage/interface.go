// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package toolstorage

import (
	"io"

	"github.com/juju/juju/version"
)

// Metadata describes a Juju tools tarball.
type Metadata struct {
	Version version.Binary
	Size    int64
	SHA256  string
}

// Storage provides methods for storing and retrieving tools by version.
type Storage interface {
	// AddTools adds the tools tarball and metadata into state,
	// replacing existing metadata if any exists with the specified
	// version.
	AddTools(io.Reader, Metadata) error

	// Tools returns the Metadata and tools tarball contents
	// for the specified version if it exists, else an error
	// satisfying errors.IsNotFound.
	Tools(version.Binary) (Metadata, io.ReadCloser, error)

	// AllMetadata returns metadata for the full list of tools in
	// the catalogue.
	AllMetadata() ([]Metadata, error)

	// Metadata returns the Metadata for the specified version
	// if it exists, else an error satisfying errors.IsNotFound.
	Metadata(v version.Binary) (Metadata, error)

	// RemoveInvalid will remove all tools with invalid metadata. This
	// exists because we had a bug that would allow tools with invalid
	// metadata to be entered, which would render "AllMetadata" unusable.
	//
	// NOTE(axw) this should not be carried over to 2.0. The upgrade step
	// will be run when upgrading to the first version of the 1.25 series
	// that is allowed to upgrade to 2.0; after that the issue should not
	// occur.
	RemoveInvalid() error
}

// StorageCloser extends the Storage interface with a Close method.
type StorageCloser interface {
	Storage
	Close() error
}
