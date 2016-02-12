// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmstore

import (
	"io"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource"
)

// Operations exposes the charm store operations needed by the Juju
// resources feature.
type Operations interface {
	// GetResource returns a reader for the resource's data. That data
	// is streamed from the charm store. It will also be stored in
	// the cache, if one is set up.
	GetResource(cURL *charm.URL, name string) (resource.Resource, io.ReadCloser, error)
}

// NewOperations returns a new set of charm store operations.
func NewOperations(deps NewOperationsDeps, cache EntityCache) (Operations, error) {
	if deps == nil {
		return nil, errors.Errorf("missing deps")
	}
	cfo := cacheForOperations{
		EntityCache: cache,
	}
	ops := &operations{
		operationsDeps: deps,
		cache:          cfo,
	}
	return ops, nil
}

// NewOperationsDeps are the dependencies required by NewOperations().
type NewOperationsDeps interface {
	// NewClient returns a new charm store client to use in operations.
	NewClient() (Client, error)
}

// operations is the Operations implementation returned
// by NewOperations().
type operations struct {
	operationsDeps
	cache cacheForOperations
}

// GetResource returns a reader for the resource's data. That data is
// streamed from the charm store.
//
// If a cache is set up then the resource is read from there. If the
// resource info is not in the cache then errors.NotFound is returned.
// If the resource data is not in the cache then it is read from the
// charm store. In that case it will also be cached.
func (ops operations) GetResource(cURL *charm.URL, name string) (resource.Resource, io.ReadCloser, error) {
	res, reader, err := ops.cache.get(name)
	if err != nil {
		return resource.Resource{}, nil, errors.Trace(err)
	}
	if reader != nil {
		// Both the info *and* the data were found in the cache.
		return res, reader, nil
	}

	// Otherwise, just the info was found in the cache. So we read the
	// data from the charm store through a new client and set the data
	// for the resource in the cache.

	if res.Origin != charmresource.OriginStore {
		return resource.Resource{}, nil, errors.NotFoundf("resource %q", res.Name)
	}

	client, err := ops.NewClient()
	if err != nil {
		return resource.Resource{}, nil, errors.Trace(err)
	}
	defer client.Close()

	reader, err = client.GetResource(cURL, res.Name, res.Revision)
	if err != nil {
		return resource.Resource{}, nil, errors.Trace(err)
	}

	res, reader, err = ops.cache.set(res.Resource, reader)
	if err != nil {
		return resource.Resource{}, nil, errors.Trace(err)
	}

	return res, reader, nil
}

type operationsDeps interface {
	// NewClient returns a new charm store client to use in operations.
	NewClient() (Client, error)
}
