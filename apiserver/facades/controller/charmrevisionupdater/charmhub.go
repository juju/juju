// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisionupdater

import (
	"context"
	"strconv"
	"time"

	"github.com/juju/charm/v8/resource"
	"github.com/juju/errors"
	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/charmhub/transport"
	"github.com/juju/juju/state"
)

const (
	// TODO(benhoyt) - also add caching and retries
	charmhubAPITimeout = 10 * time.Second
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

// createCharmhubClient creates a new charmhub Client based on this model's
// config.
func createCharmhubClient(st *state.State) (*charmhub.Client, error) {
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	modelConfig, err := model.Config()
	if err != nil {
		return nil, errors.Trace(err)
	}
	url, _ := modelConfig.CharmHubURL()
	config, err := charmhub.CharmHubConfigFromURL(url, logger.Child("charmhub"))
	if err != nil {
		return nil, errors.Trace(err)
	}
	client, err := charmhub.NewClient(config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return client, nil
}

// charmhubLatestCharmInfo fetches the latest information about the given
// charms from charmhub's "charm_refresh" API.
func charmhubLatestCharmInfo(client *charmhub.Client, ids []charmhubID) ([]charmhubResult, error) {
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
		logger.Infof("TODO charmhubLatestCharmInfo cfg %d = %#v", i, cfg)
		cfgs[i] = cfg
	}
	config := charmhub.RefreshMany(cfgs...)

	ctx, cancel := context.WithTimeout(context.Background(), charmhubAPITimeout)
	defer cancel()
	responses, err := client.Refresh(ctx, config)
	if err != nil {
		return nil, errors.Trace(err)
	}

	results := make([]charmhubResult, len(responses))
	for i, response := range responses {
		logger.Infof("TODO charmhubLatestCharmInfo response %d = %#v", i, response)
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
	revision, err := strconv.Atoi(response.Entity.Version)
	if err != nil || revision <= 0 {
		return charmhubResult{
			error: errors.NotValidf("entity version %q", response.Entity.Version),
		}
	}
	var resources []resource.Resource
	for _, r := range response.Entity.Resources {
		fingerprint, err := resource.ParseFingerprint(r.Download.HashSHA384)
		if err != nil {
			logger.Errorf("invalid resource fingerprint %q", r.Download.HashSHA384)
			continue
		}
		typ, err := resource.ParseType(r.Type)
		if err != nil {
			logger.Errorf("invalid resource type %q", r.Type)
			continue
		}
		resource := resource.Resource{
			Meta: resource.Meta{
				Name: r.Name,
				Type: typ,
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
		revision:  revision,
		resources: resources,
	}
}
