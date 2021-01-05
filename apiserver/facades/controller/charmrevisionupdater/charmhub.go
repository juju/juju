// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// TODO(benhoyt) - also add caching and retries, like we do with charmstore

package charmrevisionupdater

import (
	"context"
	"time"

	"github.com/juju/charm/v9/resource"
	"github.com/juju/errors"

	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/charmhub/transport"
)

// charmhubID holds identifying information for several charms for a
// charmhubLatestCharmInfo call.
type charmhubID struct {
	id       string
	revision int
	channel  string
	os       string
	series   string
	arch     string
}

// charmhubResult is the type charmhubLatestCharmInfo returns: information
// about a charm revision and its resources.
type charmhubResult struct {
	name      string
	timestamp time.Time
	revision  int
	resources []resource.Resource
	error     error
}

// CharmhubRefreshClient is an interface for the methods of the charmhub
// client that we need.
type CharmhubRefreshClient interface {
	Refresh(ctx context.Context, config charmhub.RefreshConfig) ([]transport.RefreshResponse, error)
}

// charmhubLatestCharmInfo fetches the latest information about the given
// charms from charmhub's "charm_refresh" API.
func charmhubLatestCharmInfo(client CharmhubRefreshClient, ids []charmhubID) ([]charmhubResult, error) {
	cfgs := make([]charmhub.RefreshConfig, len(ids))
	for i, id := range ids {
		platform := charmhub.RefreshPlatform{
			Architecture: id.arch,
			OS:           id.os,
			Series:       id.series,
		}
		cfg, err := charmhub.RefreshOne(id.id, id.revision, id.channel, platform)
		if err != nil {
			return nil, errors.Trace(err)
		}
		cfgs[i] = cfg
	}
	config := charmhub.RefreshMany(cfgs...)

	ctx, cancel := context.WithTimeout(context.TODO(), charmhub.RefreshTimeout)
	defer cancel()
	responses, err := client.Refresh(ctx, config)
	if err != nil {
		return nil, errors.Trace(err)
	}

	results := make([]charmhubResult, len(responses))
	for i, response := range responses {
		results[i] = refreshResponseToCharmhubResult(response)
	}
	return results, nil
}

// refreshResponseToCharmhubResult converts a raw RefreshResponse from the
// charmhub API into a charmhubResult.
func refreshResponseToCharmhubResult(response transport.RefreshResponse) charmhubResult {
	if response.Error != nil {
		return charmhubResult{
			error: errors.Errorf("charmhub API error %s: %s", response.Error.Code, response.Error.Message),
		}
	}
	var resources []resource.Resource
	for _, r := range response.Entity.Resources {
		fingerprint, err := resource.ParseFingerprint(r.Download.HashSHA384)
		if err != nil {
			logger.Warningf("invalid resource fingerprint %q: %v", r.Download.HashSHA384, err)
			continue
		}
		typ, err := resource.ParseType(r.Type)
		if err != nil {
			logger.Warningf("invalid resource type %q: %v", r.Type, err)
			continue
		}
		resource := resource.Resource{
			Meta: resource.Meta{
				Name:        r.Name,
				Type:        typ,
				Path:        r.Filename,
				Description: r.Description,
			},
			Origin:      resource.OriginStore,
			Revision:    r.Revision,
			Fingerprint: fingerprint,
			Size:        int64(r.Download.Size),
		}
		resources = append(resources, resource)
	}
	return charmhubResult{
		name:      response.Name,
		timestamp: response.Entity.CreatedAt,
		revision:  response.Entity.Revision,
		resources: resources,
	}
}
