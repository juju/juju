// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudimagemetadata

import (
	jujutxn "github.com/juju/txn"

	"github.com/juju/juju/mongo"
)

// MetadataAttributes contains cloud image metadata attributes.
type MetadataAttributes struct {
	// Stream contains reference to a particular stream,
	// for e.g. "daily" or "released"
	Stream string

	// Region is the name of cloud region associated with the image.
	Region string

	// Series is Os version, for e.g. "quantal".
	Series string

	// Arch is the architecture for this cloud image, for e.g. "amd64"
	Arch string

	// VirtualType contains the type of the cloud image, for e.g. "pv", "hvm". "kvm".
	VirtualType string

	// RootStorageType contains type of root storage, for e.g. "ebs", "instance".
	RootStorageType string

	// RootStorageSize contains size of root storage in gigabytes (GB).
	RootStorageSize *uint64

	// Source describes where this image is coming from: is it public? custom?
	Source SourceType
}

// Metadata describes a cloud image metadata.
type Metadata struct {
	MetadataAttributes

	// ImageId contains image identifier.
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
	// Returned result is grouped by source type and ordered by date created.
	FindMetadata(criteria MetadataFilter) (map[SourceType][]Metadata, error)
}

// DataStore exposes data store operations for use by the cloud image metadata package.
type DataStore interface {

	// RunTransaction runs desired transactions against this data source..
	RunTransaction(jujutxn.TransactionSource) error

	// GetCollection retrieves desired collection from this data source.
	GetCollection(name string) (collection mongo.Collection, closer func())
}
