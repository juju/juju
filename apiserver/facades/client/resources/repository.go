// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"

	"github.com/juju/charm/v8"
	charmresource "github.com/juju/charm/v8/resource"
	csparams "github.com/juju/charmrepo/v6/csclient/params"
	"github.com/juju/errors"

	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/charmhub/transport"
	"github.com/juju/juju/charmstore"
	corecharm "github.com/juju/juju/core/charm"
)

type NewCharmRepository interface {
	ResolveResources(resources []charmresource.Resource) ([]charmresource.Resource, error)
}

// NOTE: There maybe a better way to do this.  Juju's charmhub package is equivalent
// to charmstore.client.  Juju's charmstore package is what the charmHubClient is doing
// here, making calls to the charmhub and returning data in a format that the facade
// would like to see.

// CharmID encapsulates data for identifying a unique charm in a charm repository.
type CharmID struct {
	// URL is the url of the charm.
	URL *charm.URL

	// Origin holds the original source of a charm, including it's channel.
	Origin corecharm.Origin

	// Metadata is optional extra information about a particular model's
	// "in-theatre" use use of the charm.
	Metadata map[string]string
}

type CharmHub interface {
	Refresh(ctx context.Context, config charmhub.RefreshConfig) ([]transport.RefreshResponse, error)
}

// TODO hml 2020-12-2
// charmHubClient needs to be "caching" like the charmstoreclient is??
type charmHubClient struct {
	client CharmHub
	id     CharmID
}

// ResolveResources, looks at the provided, charmhub and backend (already downloaded)
// resources to determine which to use.  Provided (uploaded) take precedence.  If
// charmhub has a newer resource than the back end, use that.
// TODO: (hml) 2020-12-03
// this logic will need refinement as work to incorporate charmhub resources continues.
// Right now, just take the uploaded resources and add charmhub resources where resource
// is missing
func (ch *charmHubClient) ResolveResources(uploadedResources []charmresource.Resource) ([]charmresource.Resource, error) {
	chResources, err := ch.listResources()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, res := range uploadedResources {
		chResources[res.Name] = res
	}
	results := make([]charmresource.Resource, len(chResources))
	var i int
	for _, res := range chResources {
		results[i] = res
		i += 1
	}
	return results, nil
}

// listResources composes, a map of details for each of the charm's
// resources. Those details are those associated with the specific
// charm revision. They include the resource's metadata and revision.
// Found via the CharmHub api.
func (ch *charmHubClient) listResources() (map[string]charmresource.Resource, error) {
	cfg, err := charmhub.DownloadOneFromChannel(ch.id.URL.Name, ch.id.Origin.Channel.String(), charmhub.RefreshPlatform(ch.id.Origin.Platform))
	if err != nil {
		return nil, errors.Trace(err)
	}

	refreshResp, err := ch.client.Refresh(context.TODO(), cfg)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(refreshResp) == 0 {
		return nil, errors.Errorf("no download refresh responses received")
	}
	resp := refreshResp[0]

	if resp.Error != nil {
		return nil, errors.Trace(errors.New(resp.Error.Message))
	}
	results := make(map[string]charmresource.Resource, len(resp.Entity.Resources))
	for _, v := range resp.Entity.Resources {
		var err error
		results[v.Name], err = resourceFromRevision(v)
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
	r := charmresource.Resource{
		Meta: charmresource.Meta{
			Name: rev.Name,
			Type: resType,
		},
		Origin:   charmresource.OriginStore,
		Revision: rev.Revision,
		Size:     int64(rev.Download.Size),
	}
	return r, nil
}

// CharmStore exposes the functionality of the charm store as needed here.
type CharmStore interface {
	// ListResources composes, for each of the identified charms, the
	// list of details for each of the charm's resources. Those details
	// are those associated with the specific charm revision. They
	// include the resource's metadata and revision.
	ListResources([]charmstore.CharmID) ([][]charmresource.Resource, error)

	// ResourceInfo returns the metadata for the given resource.
	ResourceInfo(charmstore.ResourceRequest) (charmresource.Resource, error)
}

type charmStoreClient struct {
	client CharmStore
	id     CharmID
}

func (cs *charmStoreClient) ResolveResources(resources []charmresource.Resource) ([]charmresource.Resource, error) {
	storeResources, err := cs.resourcesFromCharmstore()
	if err != nil {
		return nil, err
	}
	resolved, err := cs.resolveResources(resources, storeResources)
	if err != nil {
		return nil, err
	}
	// TODO(ericsnow) Ensure that the non-upload resource revisions
	// match a previously published revision set?
	return resolved, nil
}

// resourcesFromCharmstore gets the info for the charm's resources in
// the charm backend. If the charm URL has a revision then that revision's
// resources are returned. Otherwise the latest info for each of the
// resources is returned.
func (cs *charmStoreClient) resourcesFromCharmstore() (map[string]charmresource.Resource, error) {
	results, err := cs.listResources([]CharmID{cs.id})
	if err != nil {
		return nil, errors.Trace(err)
	}
	storeResources := make(map[string]charmresource.Resource)
	if len(results) != 0 {
		for _, res := range results[0] {
			storeResources[res.Name] = res
		}
	}
	return storeResources, nil
}

// resolveResources determines the resource info that should actually
// be stored on the controller. That decision is based on the provided
// resources along with those in the charm backend (if any).
func (cs *charmStoreClient) resolveResources(
	resources []charmresource.Resource,
	storeResources map[string]charmresource.Resource,
) ([]charmresource.Resource, error) {
	allResolved := make([]charmresource.Resource, len(resources))
	copy(allResolved, resources)
	for i, res := range resources {
		// Note that incoming "upload" resources take precedence over
		// ones already known to the controller, regardless of their
		// origin.
		if res.Origin != charmresource.OriginStore {
			continue
		}

		resolved, err := cs.resolveStoreResource(res, storeResources)
		if err != nil {
			return nil, errors.Trace(err)
		}
		allResolved[i] = resolved
	}
	return allResolved, nil
}

// resolveStoreResource selects the resource info to use. It decides
// between the provided and latest info based on the revision.
func (cs *charmStoreClient) resolveStoreResource(
	res charmresource.Resource,
	storeResources map[string]charmresource.Resource,
) (charmresource.Resource, error) {
	storeRes, ok := storeResources[res.Name]
	if !ok {
		// This indicates that AddPendingResources() was called for
		// a resource the charm backend doesn't know about (for the
		// relevant charm revision).
		return res, nil
	}

	if res.Revision < 0 {
		// The caller wants to use the charm backend info.
		return storeRes, nil
	}
	if res.Revision == storeRes.Revision {
		// We don't worry about if they otherwise match. Only the
		// revision is significant here. So we use the info from the
		// charm backend since it is authoritative.
		return storeRes, nil
	}
	if res.Fingerprint.IsZero() {
		// The caller wants resource info from the charm backend, but with
		// a different resource revision than the one associated with
		// the charm in the backend.
		req := charmstore.ResourceRequest{
			Charm:    cs.id.URL,
			Channel:  csparams.Channel(cs.id.Origin.Channel.String()),
			Name:     res.Name,
			Revision: res.Revision,
		}
		storeRes, err := cs.client.ResourceInfo(req)
		if err != nil {
			return storeRes, errors.Trace(err)
		}
		return storeRes, nil
	}
	// The caller fully-specified a resource with a different resource
	// revision than the one associated with the charm in the backend. So
	// we use the provided info as-is.
	return res, nil
}

// ListResources composes, for each of the identified charms, the
// list of details for each of the charm's resources. Those details
// are those associated with the specific charm revision. They
// include the resource's metadata and revision.
func (cs *charmStoreClient) listResources(charmIDs []CharmID) ([][]charmresource.Resource, error) {
	chIDs := make([]charmstore.CharmID, len(charmIDs))
	for i, v := range charmIDs {
		chIDs[i] = charmstore.CharmID{
			URL:     v.URL,
			Channel: csparams.Channel(v.Origin.Channel.String()),
		}
	}
	return cs.client.ListResources(chIDs)
}

type localClient struct {
}

func (lc *localClient) ResolveResources(resources []charmresource.Resource) ([]charmresource.Resource, error) {
	var resolved []charmresource.Resource
	for _, res := range resources {
		resolved = append(resolved, charmresource.Resource{
			Meta:   res.Meta,
			Origin: charmresource.OriginUpload,
		})
	}
	return resolved, nil
}
