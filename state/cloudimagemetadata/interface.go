// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudimagemetadata

import (
	"io"

	"github.com/juju/juju/version"
)

// Metadata describes a Juju cloud images metadata.
type Metadata struct {
	Version     version.Binary
	Storage     string
	VirtType    string
	Arch        string
	RegionAlias string
	RegionName  string
	Endpoint    string
	Stream      string
}

// Storage provides methods for storing and retrieving cloud images.
type Storage interface {
	// AddMetadata adds cloud images metadata into state,
	AddMetadata(io.Reader, Metadata) error

	// AllMetadata returns metadata for the full list of cloud images in
	// the catalogue.
	AllMetadata() ([]Metadata, error)

	// Metadata returns the Metadata for the specified stream, version
	// and arch if it exists, else an error satisfying errors.IsNotFound.
	Metadata(stream string, v version.Binary, arch string) (Metadata, error)
}

// StorageCloser extends the Storage interface with a Close method.
type StorageCloser interface {
	Storage
	Close() error
}
