// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudimagemetadata

import (
	"io"

	"github.com/juju/juju/version"
)

// Metadata describes a Juju cloud images tarball.
type Metadata struct {
	Version version.Binary
	Size    int64
	SHA256  string
}

// Storage provides methods for storing and retrieving tools by version.
type Storage interface {
	// AddCloudImages adds cloud images tarball and metadata into state,
	// replacing existing metadata if any exists with the specified
	// version.
	AddCloudImages(io.Reader, Metadata) error

	// CloudImages returns the Metadata and cloud images tarball contents
	// for the specified version if it exists, else an error
	// satisfying errors.IsNotFound.
	CloudImages(version.Binary) (Metadata, io.ReadCloser, error)

	// AllMetadata returns metadata for the full list of cloud images in
	// the catalogue.
	AllMetadata() ([]Metadata, error)

	// Metadata returns the Metadata for the specified version
	// if it exists, else an error satisfying errors.IsNotFound.
	Metadata(v version.Binary) (Metadata, error)
}

// StorageCloser extends the Storage interface with a Close method.
type StorageCloser interface {
	Storage
	Close() error
}
