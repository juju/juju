// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudimagemetadata

import "time"

// CustomSource represents a source type indicating user-defined metadata for cloud images.
// This is used to differentiate "cached" metadata from simple stream or other metadata source
// from user-defined metadata.
//
// Custom metadata doesn't expire and are migrated with any model moved from the controller.
const CustomSource = "custom"

// MetadataAttributes contains cloud image metadata attributes.
type MetadataAttributes struct {
	// Stream contains reference to a particular stream,
	// for e.g. "daily" or "released"
	Stream string

	// Region is the name of cloud region associated with the image.
	Region string

	// Version is OS version, for e.g. "22.04".
	Version string

	// Arch represents the architecture of the cloud image, for example, "amd64" or "arm64".
	Arch string

	// VirtType contains virtualisation type of the cloud image, for e.g. "pv", "hvm". "kvm".
	VirtType string

	// RootStorageType contains type of root storage, for e.g. "ebs", "instance".
	RootStorageType string

	// RootStorageSize contains size of root storage in gigabytes (GB).
	RootStorageSize *uint64

	// Source describes where this image is coming from: is it public? custom?
	Source string
}

// Metadata describes a cloud image metadata.
type Metadata struct {
	MetadataAttributes

	// Priority is an importance factor for image metadata.
	// Higher number means higher priority.
	// This will allow to sort metadata by importance.
	Priority int

	// ImageID contains image identifier.
	ImageID string

	// CreationTime contains the time and date the image was created. This
	// is populated when the Metadata is saved.
	CreationTime time.Time
}

// MetadataFilter contains all metadata attributes that allows to find a particular
// cloud image metadata. Since size and source are not discriminating attributes
// for cloud image metadata, they are not included in search criteria.
type MetadataFilter struct {
	// Region stores metadata region.
	Region string

	// Versions stores all desired versions.
	Versions []string

	// Arches stores all desired architectures.
	Arches []string

	// Stream can be "" or "released" for the default "released" stream,
	// or "daily" for daily images, or any other stream that the available
	// simplestreams metadata supports.
	Stream string

	// VirtType stores virtualisation type.
	VirtType string

	// RootStorageType stores storage type.
	RootStorageType string

	// ImageID stores the image ID.
	ImageID string
}
