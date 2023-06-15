// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// TODO(benhoyt) - also add caching and retries

package charmrevisionupdater

import (
	"context"
	"time"

	"github.com/juju/charm/v11/resource"
	"github.com/juju/errors"

	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/charmhub/transport"
	"github.com/juju/juju/core/charm/metrics"
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
	metrics  map[metrics.MetricKey]string
	// Required for charmhub only.  instanceKey is a unique string associated
	// with the application. To assist with keeping KPI data in charmhub, it
	// must be the same for every charmhub Refresh action related to an
	// application. Create with the charmhub.CreateInstanceKey method.
	// LP: 1944582
	instanceKey string
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
	RefreshWithRequestMetrics(ctx context.Context, config charmhub.RefreshConfig, metrics map[metrics.MetricKey]map[metrics.MetricKey]string) ([]transport.RefreshResponse, error)
	RefreshWithMetricsOnly(ctx context.Context, metrics map[metrics.MetricKey]map[metrics.MetricKey]string) error
}

// charmhubLatestCharmInfo fetches the latest information about the given
// charms from charmhub's "charm_refresh" API.
func charmhubLatestCharmInfo(client CharmhubRefreshClient, metrics map[metrics.MetricKey]map[metrics.MetricKey]string, ids []charmhubID, now time.Time) ([]charmhubResult, error) {
	cfgs := make([]charmhub.RefreshConfig, len(ids))
	for i, id := range ids {
		base := charmhub.RefreshBase{
			Architecture: id.arch,
			Name:         id.os,
			Channel:      id.series,
		}
		cfg, err := charmhub.RefreshOne(id.instanceKey, id.id, id.revision, id.channel, base)
		if err != nil {
			return nil, errors.Trace(err)
		}
		cfg, err = charmhub.AddConfigMetrics(cfg, id.metrics)
		if err != nil {
			return nil, errors.Trace(err)
		}
		cfgs[i] = cfg
	}
	config := charmhub.RefreshMany(cfgs...)

	ctx, cancel := context.WithTimeout(context.TODO(), charmhub.RefreshTimeout)
	defer cancel()
	responses, err := client.RefreshWithRequestMetrics(ctx, config, metrics)
	if err != nil {
		return nil, errors.Trace(err)
	}

	results := make([]charmhubResult, len(responses))
	for i, response := range responses {
		results[i] = refreshResponseToCharmhubResult(response, now)
	}
	return results, nil
}

// refreshResponseToCharmhubResult converts a raw RefreshResponse from the
// charmhub API into a charmhubResult.
func refreshResponseToCharmhubResult(response transport.RefreshResponse, now time.Time) charmhubResult {
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
		res := resource.Resource{
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
		resources = append(resources, res)
	}
	return charmhubResult{
		name:      response.Name,
		timestamp: now,
		revision:  response.Entity.Revision,
		resources: resources,
	}
}
