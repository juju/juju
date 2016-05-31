// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

// TODO(ericsnow) Move this file elsewhere?
//  (e.g. top-level resource pkg, charm/resource)

import (
	"io"

	"github.com/juju/errors"

	"github.com/juju/juju/resource"
)

// OpenedResourceClient exposes the API functionality needed by OpenResource.
type OpenedResourceClient interface {
	// GetResource returns the resource info and content for the given
	// name (and unit-implied service).
	GetResource(resourceName string) (resource.Resource, io.ReadCloser, error)
}

// OpenedResource wraps the resource info and reader returned
// from the API.
type OpenedResource struct {
	resource.Resource
	io.ReadCloser
}

// OpenResource opens the identified resource using the provided client.
func OpenResource(name string, client OpenedResourceClient) (*OpenedResource, error) {
	info, reader, err := client.GetResource(name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	or := &OpenedResource{
		Resource:   info,
		ReadCloser: reader,
	}
	return or, nil
}

// Content returns the "content" for the opened resource.
func (or OpenedResource) Content() Content {
	return Content{
		Data:        or.ReadCloser,
		Size:        or.Size,
		Fingerprint: or.Fingerprint,
	}
}

// Info returns the info for the opened resource.
func (or OpenedResource) Info() resource.Resource {
	return or.Resource
}
