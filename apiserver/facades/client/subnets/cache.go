// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets

import (
	"context"
	"strings"

	"github.com/juju/errors"

	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	providercommon "github.com/juju/juju/internal/provider/common"
	"github.com/juju/juju/rpc/params"
)

func allZones(ctx context.Context, api Backing, logger corelogger.Logger) (params.ZoneResults, error) {
	var results params.ZoneResults

	zonesAsString := func(zones network.AvailabilityZones) string {
		results := make([]string, len(zones))
		for i, zone := range zones {
			results[i] = zone.Name()
		}
		return `"` + strings.Join(results, `", "`) + `"`
	}

	// Try fetching cached zones first.
	zones, err := api.AvailabilityZones()
	if err != nil {
		return results, errors.Trace(err)
	}

	if len(zones) == 0 {
		// This is likely the first time we're called.
		// Fetch all zones from the provider and update.
		zones, err = updateZones(ctx, api)
		if err != nil {
			return results, errors.Annotate(err, "cannot update known zones")
		}
		logger.Tracef(context.TODO(),
			"updated the list of known zones from the model: %s", zonesAsString(zones),
		)
	} else {
		logger.Tracef(context.TODO(), "using cached list of known zones: %s", zonesAsString(zones))
	}

	results.Results = make([]params.ZoneResult, len(zones))
	for i, zone := range zones {
		results.Results[i].Name = zone.Name()
		results.Results[i].Available = zone.Available()
	}
	return results, nil
}

// updateZones attempts to retrieve all availability zones from the environment
// provider (if supported) and then updates the persisted list of zones in
// state, returning them as well on success.
func updateZones(ctx context.Context, api Backing) (network.AvailabilityZones, error) {
	zoned, err := zonedEnviron(ctx, api)
	if err != nil {
		return nil, errors.Trace(err)
	}
	zones, err := zoned.AvailabilityZones(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if err := api.SetAvailabilityZones(zones); err != nil {
		return nil, errors.Trace(err)
	}
	return zones, nil
}

// zonedEnviron returns a providercommon.ZonedEnviron instance from the current
// model config. If the model does not support zones, an error satisfying
// errors.IsNotSupported() will be returned.
func zonedEnviron(ctx context.Context, api Backing) (providercommon.ZonedEnviron, error) {
	env, err := environs.GetEnviron(ctx, api, environs.NoopCredentialInvalidator(), environs.New)
	if err != nil {
		return nil, errors.Annotate(err, "opening environment")
	}
	if zonedEnv, ok := env.(providercommon.ZonedEnviron); ok {
		return zonedEnv, nil
	}
	return nil, errors.NotSupportedf("availability zones")
}
