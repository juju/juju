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

type CharmHubInfo interface {
	Info(ctx context.Context, name string) (transport.InfoResponse, error)
}

type charmHubClient struct {
	infoClient CharmHubInfo
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
			return resourceFromRevision(v.Resources)
		}
	}
	return nil, nil
}

func matchChannel(one corecharm.Channel, two transport.Channel) bool {
	return one.String() == two.Name
}

func resourceFromRevision(revs []transport.ResourceRevision) ([]charmresource.Resource, error) {
	result := make([]charmresource.Resource, len(revs))
	for i, v := range revs {
		resType, err := charmresource.ParseType(v.Type)
		if err != nil {
			return nil, errors.Trace(err)
		}
		result[i] = charmresource.Resource{
			Meta: charmresource.Meta{
				Name: v.Name,
				Type: resType,
				Path: v.Download.URL,
			},
			Origin:   charmresource.OriginUpload,
			Revision: v.Revision,
			// TODO (hml)
			// Convert hash 384 to fingerprint.
			// Should we do this here?
			//Fingerprint: charmresource.Fingerprint{hash.Fingerprint{}},
			Size: int64(v.Download.Size),
		}
	}
	return result, nil
}

// ResourceInfo returns the metadata for the given resource.
func (ch *charmHubClient) ResourceInfo(_ charmstore.ResourceRequest) (charmresource.Resource, error) {
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
