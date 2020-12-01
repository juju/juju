// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"

	charmresource "github.com/juju/charm/v8/resource"
	"github.com/juju/errors"

	"github.com/juju/juju/charmhub/transport"
	"github.com/juju/juju/charmstore"
	corecharm "github.com/juju/juju/core/charm"
)

type CharmHub interface {
	Info(ctx context.Context, name string) (transport.InfoResponse, error)
	ListResourceRevisions(ctx context.Context, charm, resource string) ([]transport.ResourceRevision, error)
}

type charmHubClient struct {
	infoClient CharmHub
}

// ListResources composes, for each of the identified charms, the
// list of details for each of the charm's resources. Those details
// are those associated with the specific charm revision. They
// include the resource's metadata and revision.
func (ch *charmHubClient) ListResources(charmIDs []charmstore.CharmID) ([][]charmresource.Resource, error) {
	results := make([][]charmresource.Resource, len(charmIDs))
	for i, id := range charmIDs {
		info, err := ch.infoClient.Info(context.TODO(), id.URL.Name)
		if err != nil {
			return nil, errors.Trace(err)
		}
		channel, err := corecharm.ParseChannel(string(id.Channel))
		if err != nil {
			return nil, errors.Trace(err)
		}
		result, err := parseResources(channel, info)
		if err != nil {
			return nil, errors.Trace(err)
		}
		results[i] = result
	}
	return results, nil
}

func parseResources(channel corecharm.Channel, info transport.InfoResponse) ([]charmresource.Resource, error) {
	for _, v := range info.ChannelMap {
		if matchChannel(channel, v.Channel) {
			return resourcesFromRevision(v.Resources)
		}
	}
	return nil, nil
}

func matchChannel(one corecharm.Channel, two transport.Channel) bool {
	return one.String() == two.Name
}

func resourcesFromRevision(revs []transport.ResourceRevision) ([]charmresource.Resource, error) {
	results := make([]charmresource.Resource, len(revs))
	for i, v := range revs {
		var err error
		results[i], err = resourceFromRevision(v)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	return results, nil
}

func resourceFromRevision(rev transport.ResourceRevision) (charmresource.Resource, error) {
	resType, err := charmresource.ParseType(rev.Type)
	if err != nil {
		return charmresource.Resource{}, errors.Trace(err)
	}
	return charmresource.Resource{
		Meta: charmresource.Meta{
			Name: rev.Name,
			Type: resType,
			Path: rev.Download.URL,
		},
		Origin:   charmresource.OriginUpload,
		Revision: rev.Revision,
		// TODO (hml)
		// Convert hash 384 to fingerprint.
		// Should we do this here?
		//Fingerprint: charmresource.Fingerprint{hash.Fingerprint{}},
		Size: int64(rev.Download.Size),
	}, nil
}

// ResourceInfo returns the metadata for the given resource.
func (ch *charmHubClient) ResourceInfo(req charmstore.ResourceRequest) (charmresource.Resource, error) {
	revisions, err := ch.infoClient.ListResourceRevisions(context.TODO(), req.Charm.Name, req.Name)
	if err != nil {
		return charmresource.Resource{}, errors.Trace(err)
	}
	for _, rev := range revisions {
		if req.Revision != rev.Revision {
			continue
		}
		return resourceFromRevision(rev)
	}
	return charmresource.Resource{}, nil
}

type charmStoreClient struct {
	csClient CharmStore
}

// ListResources composes, for each of the identified charms, the
// list of details for each of the charm's resources. Those details
// are those associated with the specific charm revision. They
// include the resource's metadata and revision.
func (cs *charmStoreClient) ListResources(charmIDs []charmstore.CharmID) ([][]charmresource.Resource, error) {
	return cs.csClient.ListResources(charmIDs)
}

// ResourceInfo returns the metadata for the given resource.
func (cs *charmStoreClient) ResourceInfo(req charmstore.ResourceRequest) (charmresource.Resource, error) {
	return cs.csClient.ResourceInfo(req)
}
