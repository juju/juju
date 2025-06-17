// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets

import (
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	providercommon "github.com/juju/juju/provider/common"
	"github.com/juju/juju/rpc/params"
)

// addSubnetsCache holds cached lists of spaces, zones, and subnets, used for
// fast lookups while adding subnets.
type addSubnetsCache struct {
	api            Backing
	allSpaces      map[string]string    // all defined backing space names to ids
	allZones       set.Strings          // all known provider zones
	availableZones set.Strings          // all the available zones
	allSubnets     []network.SubnetInfo // all (valid) provider subnets
	// providerIdsByCIDR maps possibly duplicated CIDRs to one or more ids.
	providerIdsByCIDR map[string]set.Strings
	// subnetsByProviderId maps unique subnet ProviderIds to pointers
	// to entries in allSubnets.
	subnetsByProviderId map[string]*network.SubnetInfo
}

func NewAddSubnetsCache(api Backing) *addSubnetsCache {
	// Empty cache initially.
	return &addSubnetsCache{
		api:                 api,
		allSpaces:           nil,
		allZones:            nil,
		availableZones:      nil,
		allSubnets:          nil,
		providerIdsByCIDR:   nil,
		subnetsByProviderId: nil,
	}
}

func allZones(ctx context.ProviderCallContext, api Backing) (params.ZoneResults, error) {
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
		logger.Tracef(
			"updated the list of known zones from the model: %s", zonesAsString(zones),
		)
	} else {
		logger.Tracef("using cached list of known zones: %s", zonesAsString(zones))
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
func updateZones(ctx context.ProviderCallContext, api Backing) (network.AvailabilityZones, error) {
	zoned, err := zonedEnviron(api)
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

type environConfigGetter struct {
	Backing
	controllerUUID string
}

func (e environConfigGetter) ControllerUUID() string {
	return e.controllerUUID
}

// zonedEnviron returns a providercommon.ZonedEnviron instance from the current
// model config. If the model does not support zones, an error satisfying
// errors.IsNotSupported() will be returned.
func zonedEnviron(api Backing) (providercommon.ZonedEnviron, error) {
	// TODO (adisazhar123): set controller UUID
	envConfGetter := environConfigGetter{
		Backing:        api,
		controllerUUID: "",
	}
	env, err := environs.GetEnviron(envConfGetter, environs.New)
	if err != nil {
		return nil, errors.Annotate(err, "opening environment")
	}
	if zonedEnv, ok := env.(providercommon.ZonedEnviron); ok {
		return zonedEnv, nil
	}
	return nil, errors.NotSupportedf("availability zones")
}
