// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudimagemetadata

// Metadata describes a cloud image metadata.
type Metadata struct {
	Storage     string
	VirtType    string
	Arch        string
	Series      string
	RegionAlias string
	RegionName  string
	Endpoint    string
	Stream      string
}

// Storage provides methods for storing and retrieving cloud image metadata.
type Storage interface {
	// AddMetadata adds cloud images metadata into state,
	AddMetadata(Metadata) error

	// AllMetadata returns metadata for the full list of cloud images in
	// the catalogue.
	AllMetadata() ([]Metadata, error)

	// FindMetadata returns the Metadata for the specified stream, series
	// and arch if it exists or an error errors.IsNotFound.
	FindMetadata(stream, series, arch string) (Metadata, error)
}

// StorageCloser extends the Storage interface with a Close method.
type StorageCloser interface {
	Storage
	Close() error
}
