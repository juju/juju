// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"

	"github.com/juju/charm/v11"
	charmresource "github.com/juju/charm/v11/resource"
	"github.com/juju/errors"

	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/charmhub/transport"
	corecharm "github.com/juju/juju/core/charm"
)

// NewCharmRepository defines methods required by the resources
// facade specific to an individual repository type.
type NewCharmRepository interface {
	ResolveResources(resources []charmresource.Resource, id CharmID) ([]charmresource.Resource, error)
}

// CharmID encapsulates data for identifying a unique charm in a charm repository.
type CharmID struct {
	// URL is the url of the charm.
	URL *charm.URL

	// Origin holds the original source of a charm, including its channel.
	Origin corecharm.Origin

	// Metadata is optional extra information about a particular model's
	// "in-theatre" use use of the charm.
	Metadata map[string]string
}

// CharmHub defines the methods required of the repository to implement
// the NewCharmRepository for charmhub.
type CharmHub interface {
	ListResourceRevisions(ctx context.Context, charm, resource string) ([]transport.ResourceRevision, error)
	Refresh(ctx context.Context, config charmhub.RefreshConfig) ([]transport.RefreshResponse, error)
}

type Logger interface {
	Debugf(string, ...interface{})
	Tracef(string, ...interface{})
	Errorf(string, ...interface{})
}

type CharmHubClient struct {
	Client CharmHub
	logger Logger
}

func NewCharmHubClient(client CharmHub, logger Logger) *CharmHubClient {
	c := &CharmHubClient{
		Client: client,
	}
	return c
}

// TODO 30-Nov-2022
// ResolveResources can be made more efficient, some choices left over from
// integration with charmstore style of working.

// ResolveResources looks at the provided charmhub and backend (already
// downloaded) resources to determine which to use. Provided (uploaded) take
// precedence. If charmhub has a newer resource than the back end, use that.
func (ch *CharmHubClient) ResolveResources(resources []charmresource.Resource, id CharmID) ([]charmresource.Resource, error) {
	revisionResources, err := ch.listResourcesIfRevisions(resources, id.URL)
	if err != nil {
		return nil, errors.Trace(err)
	}
	storeResources, err := ch.listResources(id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	for k, v := range revisionResources {
		storeResources[k] = v
	}
	resolved, err := ch.resolveResources(resources, storeResources, id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return resolved, nil
}

func (ch *CharmHubClient) listResourcesIfRevisions(resources []charmresource.Resource, curl *charm.URL) (map[string]charmresource.Resource, error) {
	results := make(map[string]charmresource.Resource, 0)
	for _, resource := range resources {
		// If not revision is specified, or the resource has already been
		// uploaded, no need to attempt to find it here.
		if resource.Revision == -1 || resource.Origin == charmresource.OriginUpload {
			continue
		}
		refreshResp, err := ch.Client.ListResourceRevisions(context.TODO(), curl.Name, resource.Name)
		if err != nil {
			return nil, errors.Annotatef(err, "refreshing charm %q", curl.String())
		}
		if len(refreshResp) == 0 {
			return nil, errors.Errorf("no download refresh responses received")
		}
		for _, res := range refreshResp {
			if res.Revision == resource.Revision {
				results[resource.Name], err = resourceFromRevision(refreshResp[0])
				if err != nil {
					return nil, errors.Trace(err)
				}
			}
		}
	}
	return results, nil
}

// listResources composes, a map of details for each of the charm's
// resources. Those details are those associated with the specific
// charm channel. They include the resource's metadata and revision.
// Found via the CharmHub api.
func (ch *CharmHubClient) listResources(id CharmID) (map[string]charmresource.Resource, error) {
	curl := id.URL
	origin := id.Origin
	refBase := charmhub.RefreshBase{
		Architecture: origin.Platform.Architecture,
		Name:         origin.Platform.OS,
		Channel:      origin.Platform.Channel,
	}
	var cfg charmhub.RefreshConfig
	var err error
	switch {
	// Do not get resource data via revision here, it is only provided if explicitly
	// asked for by resource revision.  The purpose here is to find a resource revision
	// in the channel, if one was not provided on the cli.
	case origin.ID != "":
		cfg, err = charmhub.DownloadOneFromChannel(origin.ID, origin.Channel.String(), refBase)
		if err != nil {
			ch.logger.Errorf("creating resources config for charm (%q, %q): %s", origin.ID, origin.Channel.String(), err)
			return nil, errors.Annotatef(err, "creating resources config for charm %q", curl.String())
		}
	case origin.ID == "":
		cfg, err = charmhub.DownloadOneFromChannelByName(curl.Name, origin.Channel.String(), refBase)
		if err != nil {
			ch.logger.Errorf("creating resources config for charm (%q, %q): %s", curl.Name, origin.Channel.String(), err)
			return nil, errors.Annotatef(err, "creating resources config for charm %q", curl.String())
		}
	}

	refreshResp, err := ch.Client.Refresh(context.TODO(), cfg)
	if err != nil {
		return nil, errors.Annotatef(err, "refreshing charm %q", curl.String())
	}
	if len(refreshResp) == 0 {
		return nil, errors.Errorf("no download refresh responses received")
	}
	resp := refreshResp[0]

	if resp.Error != nil {
		return nil, errors.Annotatef(errors.New(resp.Error.Message), "listing resources for charm %q", curl.String())
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

// resolveResources determines the resource info that should actually
// be stored on the controller. That decision is based on the provided
// resources along with those in the charm backend (if any).
func (c *CharmHubClient) resolveResources(resources []charmresource.Resource,
	storeResources map[string]charmresource.Resource,
	id CharmID,
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

		resolved, err := c.resolveRepositoryResources(res, storeResources, id)
		if err != nil {
			return nil, errors.Trace(err)
		}
		allResolved[i] = resolved
	}
	return allResolved, nil
}

// resolveRepositoryResources selects the resource info to use. It decides
// between the provided and latest info based on the revision.
func (c *CharmHubClient) resolveRepositoryResources(res charmresource.Resource,
	storeResources map[string]charmresource.Resource,
	id CharmID,
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
		return c.resourceInfo(id.URL, id.Origin, res.Name, res.Revision)
	}
	// The caller fully-specified a resource with a different resource
	// revision than the one associated with the charm in the backend. So
	// we use the provided info as-is.
	return res, nil
}

func (ch *CharmHubClient) resourceInfo(curl *charm.URL, origin corecharm.Origin, name string, revision int) (charmresource.Resource, error) {
	var configs []charmhub.RefreshConfig
	var err error

	// Due to async charm downloading we may not always have a charm ID to
	// use for getting resource info, however it is preferred. A charm name
	// is second best due to anticipation of charms being renamed in the
	// future. The charm url may not change, but the ID will reference the
	// new name.
	if origin.ID != "" {
		configs, err = configsByID(curl, origin, name, revision)
	} else {
		configs, err = configsByName(curl, origin, name, revision)
	}
	if err != nil {
		return charmresource.Resource{}, err
	}

	refreshResp, err := ch.Client.Refresh(context.TODO(), charmhub.RefreshMany(configs...))
	if err != nil {
		return charmresource.Resource{}, errors.Trace(err)
	}
	if len(refreshResp) == 0 {
		return charmresource.Resource{}, errors.Errorf("no download refresh responses received")
	}

	for _, resp := range refreshResp {
		if resp.Error != nil {
			return charmresource.Resource{}, errors.Trace(errors.New(resp.Error.Message))
		}

		for _, entity := range resp.Entity.Resources {
			if entity.Name == name && entity.Revision == revision {
				rfr, err := resourceFromRevision(entity)
				return rfr, err
			}
		}
	}
	return charmresource.Resource{}, errors.NotFoundf("charm resource %q at revision %d", name, revision)
}

func configsByID(curl *charm.URL, origin corecharm.Origin, name string, revision int) ([]charmhub.RefreshConfig, error) {
	var (
		configs []charmhub.RefreshConfig
		refBase = charmhub.RefreshBase{
			Architecture: origin.Platform.Architecture,
			Name:         origin.Platform.OS,
			Channel:      origin.Platform.Channel,
		}
	)
	// Get all the resources for everything and just find out which one matches.
	// The order is expected to be kept so when the response is looped through
	// we get channel, then revision.
	if sChan := origin.Channel.String(); sChan != "" {
		cfg, err := charmhub.DownloadOneFromChannel(origin.ID, sChan, refBase)
		if err != nil {
			return configs, errors.Trace(err)
		}
		configs = append(configs, cfg)
	}
	if rev := origin.Revision; rev != nil {
		cfg, err := charmhub.DownloadOneFromRevision(origin.ID, *rev)
		if err != nil {
			return configs, errors.Trace(err)
		}
		if newCfg, ok := charmhub.AddResource(cfg, name, revision); ok {
			cfg = newCfg
		}
		configs = append(configs, cfg)
	}
	if rev := curl.Revision; rev >= 0 {
		cfg, err := charmhub.DownloadOneFromRevision(origin.ID, rev)
		if err != nil {
			return configs, errors.Trace(err)
		}
		if newCfg, ok := charmhub.AddResource(cfg, name, revision); ok {
			cfg = newCfg
		}
		configs = append(configs, cfg)
	}
	return configs, nil
}

func configsByName(curl *charm.URL, origin corecharm.Origin, name string, revision int) ([]charmhub.RefreshConfig, error) {
	charmName := curl.Name
	var configs []charmhub.RefreshConfig
	// Get all the resource for everything and just find out which one matches.
	// The order is expected to be kept so when the response is looped through
	// we get channel, then revision.
	if sChan := origin.Channel.String(); sChan != "" {
		refBase := charmhub.RefreshBase{
			Architecture: origin.Platform.Architecture,
			Name:         origin.Platform.OS,
			Channel:      origin.Platform.Channel,
		}
		cfg, err := charmhub.DownloadOneFromChannelByName(charmName, sChan, refBase)
		if err != nil {
			return configs, errors.Trace(err)
		}
		configs = append(configs, cfg)
	}
	if rev := origin.Revision; rev != nil {
		cfg, err := charmhub.DownloadOneFromRevisionByName(charmName, *rev)
		if err != nil {
			return configs, errors.Trace(err)
		}
		if newCfg, ok := charmhub.AddResource(cfg, name, revision); ok {
			cfg = newCfg
		}
		configs = append(configs, cfg)
	}
	if rev := curl.Revision; rev >= 0 {
		cfg, err := charmhub.DownloadOneFromRevisionByName(charmName, rev)
		if err != nil {
			return configs, errors.Trace(err)
		}
		if newCfg, ok := charmhub.AddResource(cfg, name, revision); ok {
			cfg = newCfg
		}
		configs = append(configs, cfg)
	}
	return configs, nil
}

func resourceFromRevision(rev transport.ResourceRevision) (charmresource.Resource, error) {
	resType, err := charmresource.ParseType(rev.Type)
	if err != nil {
		return charmresource.Resource{}, errors.Trace(err)
	}
	fp, err := charmresource.ParseFingerprint(rev.Download.HashSHA384)
	if err != nil {
		return charmresource.Resource{}, errors.Trace(err)
	}
	r := charmresource.Resource{
		Meta: charmresource.Meta{
			Name:        rev.Name,
			Type:        resType,
			Path:        rev.Filename,
			Description: rev.Description,
		},
		Origin:      charmresource.OriginStore,
		Revision:    rev.Revision,
		Fingerprint: fp,
		Size:        int64(rev.Download.Size),
	}
	return r, nil
}

type localClient struct{}

func (lc *localClient) ResolveResources(resources []charmresource.Resource, _ CharmID) ([]charmresource.Resource, error) {
	var resolved []charmresource.Resource
	for _, res := range resources {
		resolved = append(resolved, charmresource.Resource{
			Meta:   res.Meta,
			Origin: charmresource.OriginUpload,
		})
	}
	return resolved, nil
}
