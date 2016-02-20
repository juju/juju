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

// Operations provides the charm-store-related functionality needed by
// the Juju resources feature.
type Operations struct {
	deps  OperationsDeps
	cache cacheForOperations
}

// NewOperations returns a new set of charm store operations.
func NewOperations(deps OperationsDeps, cache EntityCache) (*Operations, error) {
	if deps == nil {
		return nil, errors.Errorf("missing deps")
	}
	cfo := cacheForOperations{
		EntityCache: cache,
	}
	ops := &Operations{
		deps:  deps,
		cache: cfo,
	}
	return ops, nil
}

// OperationsDeps are the external dependencies of Operations.
type OperationsDeps interface {
	// NewClient returns a new charm store client to use in operations.
	NewClient() (Client, error)
}

// GetResource returns a reader for the resource's data. That data is
// streamed from the charm store.
//
// If a cache is set up then the resource is read from there. If the
// resource is not in the cache at all then errors.NotFound is returned.
// If only the resource's details are in the cache (but not the actual
// file) then the file is read from the charm store. In that case the
// cache is updated to contain the file too.
func (ops Operations) GetResource(cURL *charm.URL, name string) (resource.Resource, io.ReadCloser, error) {
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

	client, err := ops.deps.NewClient()
	if err != nil {
		return resource.Resource{}, nil, errors.Trace(err)
	}
	defer client.Close()

	reader, err = client.GetResource(cURL, res.Name, res.Revision)
	if err != nil {
		if errors.IsNotFound(err) {
			err = errors.Annotate(err, "(in the charm store)")
		}
		return resource.Resource{}, nil, errors.Trace(err)
	}

	res, reader, err = ops.cache.set(res.Resource, reader)
	if err != nil {
		return resource.Resource{}, nil, errors.Trace(err)
	}

	return res, reader, nil
}
