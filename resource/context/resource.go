// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

// TODO(ericsnow) Move this file elsewhere?
//  (e.g. top-level resource pkg, charm/resource)

import (
	"bytes"
	"io"

	"github.com/juju/errors"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource"
)

type openedResourceClient interface {
	// GetResource returns the resource info and content for the given
	// name (and unit-implied service).
	GetResource(resourceName string) (resource.Resource, io.ReadCloser, error)
}

type openedResource struct {
	resource.Resource
	io.ReadCloser
}

func openResource(name string, client openedResourceClient) (*openedResource, error) {
	info, reader, err := client.GetResource(name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	or := &openedResource{
		Resource:   info,
		ReadCloser: reader,
	}
	return or, nil
}

func (or openedResource) content() resourceContent {
	return resourceContent{
		data:        or,
		size:        or.Size,
		fingerprint: or.Fingerprint,
	}
}

// resourceContent holds a reader for the content of a resource along
// with details about that content.
type resourceContent struct {
	data        io.Reader
	size        int64
	fingerprint charmresource.Fingerprint
}

// verify ensures that the actual resource content details match
// the expected ones.
func (c resourceContent) verify(size int64, fp charmresource.Fingerprint) error {
	if size != c.size {
		return errors.Errorf("resource size does not match expected (%d != %d)", size, c.size)
	}
	if !bytes.Equal(fp.Bytes(), c.fingerprint.Bytes()) {
		return errors.Errorf("resource fingerprint does not match expected (%q != %q)", fp, c.fingerprint)
	}
	return nil
}
