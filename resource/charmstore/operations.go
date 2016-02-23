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

// GetResourceArgs holds the arguments to GetResource().
type GetResourceArgs struct {
	// Client is the charm store client to use.
	Client Client

	// EntityCache is the charm store cache to use. It is optional.
	Cache EntityCache

	// CharmURL and Name together identify the resource to get.
	CharmURL *charm.URL
	Name     string
}

func (args GetResourceArgs) validate() error {
	if args.Client == nil {
		return errors.Errorf("missing charm store client")
	}
	// FYI, args.Cache may be nil.
	if args.CharmURL == nil {
		return errors.Errorf("missing charm URL")
	}
	if args.Name == "" {
		return errors.Errorf("missing resource name")
	}
	return nil
}

// GetResource returns a reader for the resource's data. That data is
// streamed from the charm store.
//
// If a cache is set up then the resource is read from there. If the
// resource is not in the cache at all then errors.NotFound is returned.
// If only the resource's details are in the cache (but not the actual
// file) then the file is read from the charm store. In that case the
// cache is updated to contain the file too.
func GetResource(args GetResourceArgs) (resource.Resource, io.ReadCloser, error) {
	if err := args.validate(); err != nil {
		return resource.Resource{}, nil, errors.Trace(err)
	}

	cache := cacheForOperations{
		EntityCache: args.Cache,
	}

	res, reader, err := cache.get(args.Name)
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

	reader, err = args.Client.GetResource(args.CharmURL, res.Name, res.Revision)
	if errors.IsNotFound(err) {
		msg := "while getting resource from the charm store"
		return resource.Resource{}, nil, errors.Annotate(err, msg)
	}
	if err != nil {
		return resource.Resource{}, nil, errors.Trace(err)
	}

	res, reader, err = cache.set(res.Resource, reader)
	if err != nil {
		return resource.Resource{}, nil, errors.Trace(err)
	}

	return res, reader, nil
}
