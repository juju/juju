// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// ImageMetadataFilter holds filter properties used to search for image metadata.
// It amalgamates both simplestreams.MetadataLookupParams and simplestreams.LookupParams
// and adds additional properties to satisfy existing and new use cases.
type ImageMetadataFilter struct {
	// Region stores metadata region.
	Region string `json:"region,omitempty"`

	// Series stores all desired series.
	Series []string `json:"series,omitempty"`

	// Arches stores all desired architectures.
	Arches []string `json:"arches,omitempty"`

	// Stream can be "" or "released" for the default "released" stream,
	// or "daily" for daily images, or any other stream that the available
	// simplestreams metadata supports.
	Stream string `json:"stream,omitempty"`

	// VirtualType stores virtual type.
	VirtualType string `json:"virtual_type,omitempty"`

	// RootStorageType stores storage type.
	RootStorageType string `json:"root-storage-type,omitempty"`
}

// CloudImageMetadata holds cloud image metadata properties.
type CloudImageMetadata struct {
	// ImageId is an image identifier.
	ImageId string `json:"image_id"`

	// Stream contains reference to a particular stream,
	// for e.g. "daily" or "released"
	Stream string `json:"stream,omitempty"`

	// Region is the name of cloud region associated with the image.
	Region string `json:"region"`

	// Series is OS version, for e.g. "quantal".
	Series string `json:"series"`

	// Arch is the architecture for this cloud image, for e.g. "amd64"
	Arch string `json:"arch"`

	// VirtualType contains the type of the cloud image, for e.g. "pv", "hvm". "kvm".
	VirtualType string `json:"virtual_type,omitempty"`

	// RootStorageType contains type of root storage, for e.g. "ebs", "instance".
	RootStorageType string `json:"root_storage_type,omitempty"`

	// RootStorageSize contains size of root storage in gigabytes (GB).
	RootStorageSize *uint64 `json:"root_storage_size,omitempty"`

	// Source describes where this image is coming from: is it public? custom?
	Source string `json:"source"`
}

// ListCloudImageMetadataResult holds the results of querying cloud image metadata.
type ListCloudImageMetadataResult struct {
	Result []CloudImageMetadata `json:"result"`
}

// MetadataSaveParams holds cloud image metadata details to save.
type MetadataSaveParams struct {
	Metadata []CloudImageMetadata `json:"metadata"`
}
