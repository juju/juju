// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets

import (
	"net"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/core/instance"
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

// validateSpace parses the given spaceTag and verifies it exists by looking it
// up in the cache (or populates the cache if empty).  Returns the spaceID
// associated with the space name given.
func (cache *addSubnetsCache) validateSpace(api Backing, spaceTag string) (string, error) {
	if spaceTag == "" {
		return "", errors.New("SpaceTag is required")
	}
	tag, err := names.ParseSpaceTag(spaceTag)
	if err != nil {
		return "", errors.Annotate(err, "given SpaceTag is invalid")
	}

	// Otherwise we need the cache to validate.
	if cache.allSpaces == nil {
		// Not yet cached.
		logger.Tracef("caching known spaces")

		allSpaces, err := api.AllSpaces()
		if err != nil {
			return "", errors.Annotate(err, "cannot validate given SpaceTag")
		}
		cache.allSpaces = make(map[string]string, len(allSpaces))
		for _, space := range allSpaces {
			if _, ok := cache.allSpaces[space.Name()]; ok {
				logger.Warningf("ignoring duplicated space %q", space.Name())
				continue
			}
			cache.allSpaces[space.Name()] = space.Id()
		}
	}
	if len(cache.allSpaces) == 0 {
		return "", errors.New("no spaces defined")
	}
	logger.Tracef("using cached spaces: %v", cache.allSpaces)

	spaceID, ok := cache.allSpaces[tag.Id()]
	if !ok {
		return "", errors.NotFoundf("space %q", tag.Id()) // " not found"
	}
	return spaceID, nil
}

// cacheZones populates the allZones and availableZones cache, if it's
// empty.
func (cache *addSubnetsCache) cacheZones(ctx context.ProviderCallContext) error {
	if cache.allZones != nil {
		// Already cached.
		logger.Tracef("using cached zones: %v", cache.allZones.SortedValues())
		return nil
	}

	allZones, err := allZones(ctx, cache.api)
	if err != nil {
		return errors.Annotate(err, "given Zones cannot be validated")
	}
	cache.allZones = set.NewStrings()
	cache.availableZones = set.NewStrings()
	for _, zone := range allZones.Results {
		// AllZones() does not use the Error result field, so no
		// need to check it here.
		if cache.allZones.Contains(zone.Name) {
			logger.Warningf("ignoring duplicated zone %q", zone.Name)
			continue
		}

		if zone.Available {
			cache.availableZones.Add(zone.Name)
		}
		cache.allZones.Add(zone.Name)
	}
	logger.Tracef(
		"%d known and %d available zones cached: %v",
		cache.allZones.Size(), cache.availableZones.Size(), cache.allZones.SortedValues(),
	)
	if cache.allZones.IsEmpty() {
		cache.allZones = nil
		// Cached an empty list.
		return errors.New("no zones defined")
	}
	return nil
}

// validateZones ensures givenZones are valid. When providerZones are also set,
// givenZones must be a subset of them or match exactly. With non-empty
// providerZones and empty givenZones, it returns the providerZones (i.e. trusts
// the provider to know better). When no providerZones and only givenZones are
// set, only then the cache is used to validate givenZones.
func (cache *addSubnetsCache) validateZones(ctx context.ProviderCallContext, providerZones, givenZones []string) ([]string, error) {
	givenSet := set.NewStrings(givenZones...)
	providerSet := set.NewStrings(providerZones...)

	// First check if we can validate without using the cache.
	switch {
	case providerSet.IsEmpty() && givenSet.IsEmpty():
		return nil, errors.New("Zones cannot be discovered from the provider and must be set")
	case !providerSet.IsEmpty() && givenSet.IsEmpty():
		// Use provider zones when none given.
		return providerSet.SortedValues(), nil
	case !providerSet.IsEmpty() && !givenSet.IsEmpty():
		// Ensure givenZones either match providerZones or are a
		// subset of them.
		extraGiven := givenSet.Difference(providerSet)
		if !extraGiven.IsEmpty() {
			extra := `"` + strings.Join(extraGiven.SortedValues(), `", "`) + `"`
			return nil, errors.Errorf("Zones contain zones not allowed by the provider: %s", extra)
		}
	}

	// Otherwise we need the cache to validate.
	if err := cache.cacheZones(ctx); err != nil {
		return nil, errors.Trace(err)
	}

	diffAvailable := givenSet.Difference(cache.availableZones)
	diffAll := givenSet.Difference(cache.allZones)

	if !diffAll.IsEmpty() {
		extra := `"` + strings.Join(diffAll.SortedValues(), `", "`) + `"`
		return nil, errors.Errorf("Zones contain unknown zones: %s", extra)
	}
	if !diffAvailable.IsEmpty() {
		extra := `"` + strings.Join(diffAvailable.SortedValues(), `", "`) + `"`
		return nil, errors.Errorf("Zones contain unavailable zones: %s", extra)
	}
	// All good - given zones are a subset and none are
	// unavailable.
	return givenSet.SortedValues(), nil
}

// cacheSubnets tries to get and cache once all known provider subnets. It
// handles the case when subnets have duplicated CIDRs but distinct ProviderIds.
// It also handles weird edge cases, like no CIDR and/or ProviderId set for a
// subnet.
func (cache *addSubnetsCache) cacheSubnets(ctx context.ProviderCallContext) error {
	if cache.allSubnets != nil {
		// Already cached.
		logger.Tracef("using %d cached subnets", len(cache.allSubnets))
		return nil
	}

	netEnv, err := networkingEnviron(cache.api)
	if err != nil {
		return errors.Trace(err)
	}
	subnetInfo, err := netEnv.Subnets(ctx, instance.UnknownId, nil)
	if err != nil {
		return errors.Annotate(err, "cannot get provider subnets")
	}
	logger.Tracef("got %d subnets to cache from the provider", len(subnetInfo))

	if len(subnetInfo) > 0 {
		// Trying to avoid reallocations.
		cache.allSubnets = make([]network.SubnetInfo, 0, len(subnetInfo))
	}
	cache.providerIdsByCIDR = make(map[string]set.Strings)
	cache.subnetsByProviderId = make(map[string]*network.SubnetInfo)

	for i := range subnetInfo {
		subnet := subnetInfo[i]
		cidr := subnet.CIDR
		providerId := string(subnet.ProviderId)
		logger.Tracef(
			"caching subnet with CIDR %q, ProviderId %q, Zones: %q",
			cidr, providerId, subnet.AvailabilityZones,
		)

		if providerId == "" && cidr == "" {
			logger.Warningf("found subnet with empty CIDR and ProviderId")
			// But we still save it for lookups, which will probably fail anyway.
		} else if providerId == "" {
			logger.Warningf("found subnet with CIDR %q and empty ProviderId", cidr)
			// But we still save it for lookups.
		} else {
			_, ok := cache.subnetsByProviderId[providerId]
			if ok {
				logger.Warningf(
					"found subnet with CIDR %q and duplicated ProviderId %q",
					cidr, providerId,
				)
				// We just overwrite what's there for the same id.
				// It's a weird case and it shouldn't happen with
				// properly written providers, but anyway..
			}
		}
		cache.subnetsByProviderId[providerId] = &subnet

		if ids, ok := cache.providerIdsByCIDR[cidr]; !ok {
			cache.providerIdsByCIDR[cidr] = set.NewStrings(providerId)
		} else {
			ids.Add(providerId)
			logger.Debugf(
				"duplicated subnet CIDR %q; collected ProviderIds so far: %v",
				cidr, ids.SortedValues(),
			)
			cache.providerIdsByCIDR[cidr] = ids
		}

		cache.allSubnets = append(cache.allSubnets, subnet)
	}
	logger.Tracef("%d provider subnets cached", len(cache.allSubnets))
	if len(cache.allSubnets) == 0 {
		// Cached an empty list.
		return errors.New("no subnets defined")
	}
	return nil
}

// TODO (hml) 2019-08-27
// This logic needs to be updated when auditing add-subnet and the
// subnet cache.  You need a providerId or cidr if there is only
// one of that cidr in the network.  If there are duplicate cidrs
// in the network, the providerId will be required.
//
// validateSubnet ensures either cidr or providerId is valid (not both),
// then uses the cache to validate and lookup the provider SubnetInfo for the
// subnet, if found.
func (cache *addSubnetsCache) validateSubnet(ctx context.ProviderCallContext, cidr, providerId string) (*network.SubnetInfo, error) {
	haveCidr := cidr != ""
	haveProviderId := providerId != ""

	if !haveCidr && !haveProviderId {
		return nil, errors.New("either CIDR or SubnetProviderId is required")
	} else if haveCidr && haveProviderId {
		return nil, errors.New("CIDR and SubnetProviderId cannot be both set")
	}
	if haveCidr {
		if !network.IsValidCIDR(cidr) {
			return nil, errors.Errorf("%q is not a valid CIDR", cidr)
		}
	}

	// Otherwise we need the cache to validate.
	if err := cache.cacheSubnets(ctx); err != nil {
		return nil, errors.Trace(err)
	}

	if haveCidr {
		providerIds, ok := cache.providerIdsByCIDR[cidr]
		if !ok || providerIds.IsEmpty() {
			return nil, errors.NotFoundf("subnet with CIDR %q", cidr)
		}
		if providerIds.Size() > 1 {
			ids := `"` + strings.Join(providerIds.SortedValues(), `", "`) + `"`
			return nil, errors.Errorf(
				"multiple subnets with CIDR %q: retry using ProviderId from: %s",
				cidr, ids,
			)
		}
		// A single CIDR matched.
		providerId = providerIds.Values()[0]
	}

	info, ok := cache.subnetsByProviderId[providerId]
	if !ok || info == nil {
		return nil, errors.NotFoundf(
			"subnet with CIDR %q and ProviderId %q",
			cidr, providerId,
		)
	}
	// Do last-call validation.
	if !network.IsValidCIDR(info.CIDR) {
		_, ipnet, err := net.ParseCIDR(info.CIDR)
		if err != nil && info.CIDR != "" {
			// The underlying error is not important here, just that
			// the CIDR is invalid.
			return nil, errors.Errorf(
				"subnet with CIDR %q and ProviderId %q: invalid CIDR",
				info.CIDR, providerId,
			)
		}
		if info.CIDR == "" {
			return nil, errors.Errorf(
				"subnet with ProviderId %q: empty CIDR", providerId,
			)
		}
		return nil, errors.Errorf(
			"subnet with ProviderId %q: incorrect CIDR format %q, expected %q",
			providerId, info.CIDR, ipnet.String(),
		)
	}
	return info, nil
}

// addOneSubnet validates the given arguments, using cache for lookups
// (initialized on first use), then adds it to the backing store, if successful.
func addOneSubnet(
	ctx context.ProviderCallContext, api Backing, args params.AddSubnetParams, cache *addSubnetsCache,
) error {
	subnetInfo, err := cache.validateSubnet(ctx, args.CIDR, args.SubnetProviderId)
	if err != nil {
		return errors.Trace(err)
	}
	spaceID, err := cache.validateSpace(api, args.SpaceTag)
	if err != nil {
		return errors.Trace(err)
	}
	zones, err := cache.validateZones(ctx, subnetInfo.AvailabilityZones, args.Zones)
	if err != nil {
		return errors.Trace(err)
	}

	// Try adding the subnet.
	backingInfo := networkingcommon.BackingSubnetInfo{
		ProviderId:        subnetInfo.ProviderId,
		ProviderNetworkId: subnetInfo.ProviderNetworkId,
		CIDR:              subnetInfo.CIDR,
		VLANTag:           subnetInfo.VLANTag,
		AvailabilityZones: zones,
		SpaceID:           spaceID,
	}
	if _, err := api.AddSubnet(backingInfo); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// networkingEnviron returns a environs.NetworkingEnviron instance from the
// current model config, if supported. If the model does not support
// environs.Networking, an error satisfying errors.IsNotSupported() will be
// returned.
func networkingEnviron(getter environs.EnvironConfigGetter) (environs.NetworkingEnviron, error) {
	env, err := environs.GetEnviron(getter, environs.New)
	if err != nil {
		return nil, errors.Annotate(err, "opening environment")
	}
	if netEnv, ok := environs.SupportsNetworking(env); ok {
		return netEnv, nil
	}
	return nil, errors.NotSupportedf("model networking features") // " not supported"
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

// zonedEnviron returns a providercommon.ZonedEnviron instance from the current
// model config. If the model does not support zones, an error satisfying
// errors.IsNotSupported() will be returned.
func zonedEnviron(api Backing) (providercommon.ZonedEnviron, error) {
	env, err := environs.GetEnviron(api, environs.New)
	if err != nil {
		return nil, errors.Annotate(err, "opening environment")
	}
	if zonedEnv, ok := env.(providercommon.ZonedEnviron); ok {
		return zonedEnv, nil
	}
	return nil, errors.NotSupportedf("availability zones")
}
