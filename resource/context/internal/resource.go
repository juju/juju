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

type OpenedResourceClient interface {
	// GetResource returns the resource info and content for the given
	// name (and unit-implied service).
	GetResource(resourceName string) (resource.Resource, io.ReadCloser, error)
}

type OpenedResource struct {
	resource.Resource
	io.ReadCloser
}

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

func (or OpenedResource) Content() Content {
	return Content{
		Data:        or,
		Size:        or.Size,
		Fingerprint: or.Fingerprint,
	}
}

func (or OpenedResource) Info() resource.Resource {
	return or.Resource
}
