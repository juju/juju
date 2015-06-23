// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudimagemetadata

// Metadata describes a cloud image metadata.
type Metadata struct {
	Stream          string
	Region          string
	Series          string
	Arch            string
	VirtualType     string
	RootStorageType string
}

// Storage provides methods for storing and retrieving cloud image metadata.
type Storage interface {
	// SaveMetadata adds cloud images metadata into state if it's new or
	// updates metadata if it already exists,
	SaveMetadata(Metadata) error

	// AllMetadata returns metadata for the full list of cloud images in
	// the catalogue.
	AllMetadata() ([]Metadata, error)

	// FindMetadata returns all Metadata that match specified
	// criteria or a "not found" error if none match.
	FindMetadata(criteria Metadata) ([]Metadata, error)
}

// StorageCloser extends the Storage interface with a Close method.
type StorageCloser interface {
	Storage
	Close() error
}
