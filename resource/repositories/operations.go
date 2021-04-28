// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package repositories

import (
	"io"

	"github.com/juju/charm/v8"
	charmresource "github.com/juju/charm/v8/resource"
	"github.com/juju/errors"

	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/state"
)

type ResourceRequest struct {
	// Channel is the channel from which to request the resource info.
	CharmID CharmID

	// Name is the name of the resource we're asking about.
	Name string

	// Revision is the specific revision of the resource we're asking about.
	Revision int
}

// ResourceGetter provides the functionality for getting a resource file.
type ResourceGetter interface {
	// GetResource returns a reader for the resource's data. That data
	// is streamed from the charm store. The charm's revision, if any,
	// is ignored. If the identified resource is not in the charm store
	// then errors.NotFound is returned.
	//
	// But if you write any code that assumes a NotFound error returned
	// from this method means that the resource was not found, you fail
	// basic logic.
	GetResource(ResourceRequest) (charmstore.ResourceData, error)
}

// CharmID represents the underlying charm for a given application. This
// includes both the URL and the origin.
type CharmID struct {

	// URL of the given charm, includes the reference name and a revision.
	// Old style charm URLs are also supported i.e. charmstore.
	URL *charm.URL

	// Origin holds the origin of a charm. This includes the source of the
	// charm, along with the revision and channel to identify where the charm
	// originated from.
	Origin state.CharmOrigin
}

// GetResourceArgs holds the arguments to GetResource().
type GetResourceArgs struct {
	// Client is the charm store client to use.
	Client ResourceGetter

	// EntityRepository is the charm store cache to use. It is optional.
	Repository EntityRepository

	// CharmID indicates the charm for which to get the resource.
	CharmID CharmID

	// Name is the name of the resource.
	Name string
}

func (args GetResourceArgs) validate() error {
	if args.Client == nil {
		return errors.Errorf("missing charm store client")
	}
	// FYI, args.Cache may be nil.
	if args.CharmID.URL == nil {
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

	opRepo := operationsRepository{
		repo: args.Repository,
	}

	res, reader, err := opRepo.get(args.Name)
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

	req := ResourceRequest{
		CharmID:  args.CharmID,
		Name:     res.Name,
		Revision: res.Revision,
	}
	data, err := args.Client.GetResource(req)
	// (anastasiamac 2017-05-25) This might not work all the time
	// as the error types may be lost after call to some clients, for example http.
	// But for these cases, the next block will bubble an un-annotated error up.
	if errors.IsNotFound(err) {
		msg := "while getting resource from the charm store"
		return resource.Resource{}, nil, errors.Annotate(err, msg)
	}
	if err != nil {
		return resource.Resource{}, nil, errors.Trace(err)
	}

	res, reader, err = opRepo.set(data.Resource, data, state.DoNotIncrementCharmModifiedVersion)
	if err != nil {
		return resource.Resource{}, nil, errors.Trace(err)
	}

	return res, reader, nil
}
