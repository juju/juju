// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudimagemetadata

// MetadataAttributes contains cloud image metadata attributes.
type MetadataAttributes struct {
	Stream          string
	Region          string
	Series          string
	Arch            string
	VirtualType     string
	RootStorageType string
	RootStorageSize uint64
}

// Metadata describes a cloud image metadata.
type Metadata struct {
	MetadataAttributes
	ImageId string
}

// Storage provides methods for storing and retrieving cloud image metadata.
type Storage interface {
	// SaveMetadata adds cloud images metadata into state if it's new or
	// updates metadata if it already exists,
	SaveMetadata(Metadata) error

	// FindMetadata returns all Metadata that match specified
	// criteria or a "not found" error if none match.
	// Empty criteria will return all cloud image metadata.
	FindMetadata(criteria MetadataAttributes) ([]Metadata, error)
}
