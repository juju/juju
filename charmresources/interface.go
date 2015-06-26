// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmresources

import (
	"io"
	"time"
)

type ResourceType string

const (
	// These constants represent the supported resource types.
	ResourceTypeBlob = ResourceType("blob")
	ResourceTypeZip  = ResourceType("zip")

	DefaultResourceType = ResourceTypeBlob
)

// ResourceReader is used to gain access to a resource's data bytes.
type ResourceReader struct {
	io.ReadCloser
	Resource
}

// ResourceAttributes describe a resource and are used to form
// the resource's path.
type ResourceAttributes struct {
	// Type is the resource type eg blob, deb.
	Type string

	// User is the optional user the resource is associated with.
	User string

	// Org is the optional organisation or team the resource is associated with.
	Org string

	// Stream is a category of resource eg proposed, test, released.
	Stream string

	// Series is the OS series for which the resource is applicable.
	Series string

	// PathName is the resource's main identified and is mandatory.
	PathName string

	// Revision is the version associated with the resource.
	Revision string
}

// Resource describes a stored charm resource.
type Resource struct {
	// Path is the location of the resource in a resource store.
	Path string

	// SHA384Hash is the hash of the resource data.
	SHA384Hash string

	// Size is the number of bytes in the resource data.
	Size int64

	// Created is when the resource was first stored.
	Created time.Time
}

// ResourceManager instances provide the capability to manage charm resources.
type ResourceManager interface {
	// ResourceGet returns a primary and zero or more
	// dependent resources from the specified path.
	ResourceGet(resourcePath string) ([]ResourceReader, error)

	// ResourcePut stores the resource with the specified metadata,
	// and returns the metadata with additional attributes filled in.
	ResourcePut(metadata Resource, rdr io.Reader) (Resource, error)

	// ResourceList returns resource metadata matching the specified filter.
	ResourceList(filter ResourceAttributes) ([]Resource, error)

	// ResourceDelete removes the resource at the specified path.
	ResourceDelete(resourcePath string) error
}
