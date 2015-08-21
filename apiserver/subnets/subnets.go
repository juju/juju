// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets

import (
	"fmt"
	"net"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils/set"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	providercommon "github.com/juju/juju/provider/common"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.subnets")

func init() {
	common.RegisterStandardFacade("Subnets", 1, NewAPI)
}

// API defines the methods the Subnets API facade implements.
type API interface {
	// AllZones returns all availability zones known to Juju. If a
	// zone is unusable, unavailable, or deprecated the Available
	// field will be false.
	AllZones() (params.ZoneResults, error)

	// AllSpaces returns the tags of all network spaces known to Juju.
	AllSpaces() (params.SpaceResults, error)

	// AddSubnets adds existing subnets to Juju.
	AddSubnets(args params.AddSubnetsParams) (params.ErrorResults, error)

	// ListSubnets returns the matching subnets after applying
	// optional filters.
	ListSubnets(args params.SubnetsFilters) (params.ListSubnetsResults, error)
}

// subnetsAPI implements the API interface.
type subnetsAPI struct {
	backing    common.NetworkBacking
	resources  *common.Resources
	authorizer common.Authorizer
}

// NewAPI creates a new Subnets API server-side facade with a
// state.State backing.
func NewAPI(st *state.State, res *common.Resources, auth common.Authorizer) (API, error) {
	return newAPIWithBacking(&stateShim{st: st}, res, auth)
}

// newAPIWithBacking creates a new server-side Subnets API facade with
// a common.NetworkBacking
func newAPIWithBacking(backing common.NetworkBacking, resources *common.Resources, authorizer common.Authorizer) (API, error) {
	// Only clients can access the Subnets facade.
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}
	return &subnetsAPI{
		backing:    backing,
		resources:  resources,
		authorizer: authorizer,
	}, nil
}

// AllZones is defined on the API interface.
func (api *subnetsAPI) AllZones() (params.ZoneResults, error) {
	var results params.ZoneResults

	zonesAsString := func(zones []providercommon.AvailabilityZone) string {
		results := make([]string, len(zones))
		for i, zone := range zones {
			results[i] = zone.Name()
		}
		return `"` + strings.Join(results, `", "`) + `"`
	}

	// Try fetching cached zones first.
	zones, err := api.backing.AvailabilityZones()
	if err != nil {
		return results, errors.Trace(err)
	}

	if len(zones) == 0 {
		// This is likely the first time we're called.
		// Fetch all zones from the provider and update.
		zones, err = api.updateZones()
		if err != nil {
			return results, errors.Annotate(err, "cannot update known zones")
		}
		logger.Debugf(
			"updated the list of known zones from the environment: %s", zonesAsString(zones),
		)
	} else {
		logger.Debugf("using cached list of known zones: %s", zonesAsString(zones))
	}

	results.Results = make([]params.ZoneResult, len(zones))
	for i, zone := range zones {
		results.Results[i].Name = zone.Name()
		results.Results[i].Available = zone.Available()
	}
	return results, nil
}

// AllSpaces is defined on the API interface.
func (api *subnetsAPI) AllSpaces() (params.SpaceResults, error) {
	var results params.SpaceResults

	spaces, err := api.backing.AllSpaces()
	if err != nil {
		return results, errors.Trace(err)
	}

	results.Results = make([]params.SpaceResult, len(spaces))
	for i, space := range spaces {
		// TODO(dimitern): Add a Tag() a method and use it here. Too
		// early to do it now as it will just complicate the tests.
		tag := names.NewSpaceTag(space.Name())
		results.Results[i].Tag = tag.String()
	}
	return results, nil
}

// zonedEnviron returns a providercommon.ZonedEnviron instance from
// the current environment config. If the environment does not support
// zones, an error satisfying errors.IsNotSupported() will be
// returned.
func (api *subnetsAPI) zonedEnviron() (providercommon.ZonedEnviron, error) {
	envConfig, err := api.backing.EnvironConfig()
	if err != nil {
		return nil, errors.Annotate(err, "getting environment config")
	}

	env, err := environs.New(envConfig)
	if err != nil {
		return nil, errors.Annotate(err, "opening environment")
	}
	if zonedEnv, ok := env.(providercommon.ZonedEnviron); ok {
		return zonedEnv, nil
	}
	return nil, errors.NotSupportedf("availability zones")
}

// networkingEnviron returns a environs.NetworkingEnviron instance
// from the current environment config, if supported. If the
// environment does not support environs.Networking, an error
// satisfying errors.IsNotSupported() will be returned.
func (api *subnetsAPI) networkingEnviron() (environs.NetworkingEnviron, error) {
	envConfig, err := api.backing.EnvironConfig()
	if err != nil {
		return nil, errors.Annotate(err, "getting environment config")
	}

	env, err := environs.New(envConfig)
	if err != nil {
		return nil, errors.Annotate(err, "opening environment")
	}
	if netEnv, ok := environs.SupportsNetworking(env); ok {
		return netEnv, nil
	}
	return nil, errors.NotSupportedf("environment networking features") // " not supported"
}

// updateZones attempts to retrieve all availability zones from the
// environment provider (if supported) and then updates the persisted
// list of zones in state, returning them as well on success.
func (api *subnetsAPI) updateZones() ([]providercommon.AvailabilityZone, error) {
	zoned, err := api.zonedEnviron()
	if err != nil {
		return nil, errors.Trace(err)
	}
	zones, err := zoned.AvailabilityZones()
	if err != nil {
		return nil, errors.Trace(err)
	}

	if err := api.backing.SetAvailabilityZones(zones); err != nil {
		return nil, errors.Trace(err)
	}
	return zones, nil
}

// addSubnetsCache holds cached lists of spaces, zones, and subnets,
// used for fast lookups while adding subnets.
type addSubnetsCache struct {
	api            *subnetsAPI
	allSpaces      set.Strings          // all defined backing spaces
	allZones       set.Strings          // all known provider zones
	availableZones set.Strings          // all the available zones
	allSubnets     []network.SubnetInfo // all (valid) provider subnets
	// providerIdsByCIDR maps possibly duplicated CIDRs to one or more ids.
	providerIdsByCIDR map[string]set.Strings
	// subnetsByProviderId maps unique subnet ProviderIds to pointers
	// to entries in allSubnets.
	subnetsByProviderId map[string]*network.SubnetInfo
}

func newAddSubnetsCache(api *subnetsAPI) *addSubnetsCache {
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

// validateSpace parses the given spaceTag and verifies it exists by
// looking it up in the cache (or populates the cache if empty).
func (cache *addSubnetsCache) validateSpace(spaceTag string) (*names.SpaceTag, error) {
	if spaceTag == "" {
		return nil, errors.Errorf("SpaceTag is required")
	}
	tag, err := names.ParseSpaceTag(spaceTag)
	if err != nil {
		return nil, errors.Annotate(err, "given SpaceTag is invalid")
	}

	// Otherwise we need the cache to validate.
	if cache.allSpaces == nil {
		// Not yet cached.
		logger.Debugf("caching known spaces")

		allSpaces, err := cache.api.backing.AllSpaces()
		if err != nil {
			return nil, errors.Annotate(err, "cannot validate given SpaceTag")
		}
		cache.allSpaces = set.NewStrings()
		for _, space := range allSpaces {
			if cache.allSpaces.Contains(space.Name()) {
				logger.Warningf("ignoring duplicated space %q", space.Name())
				continue
			}
			cache.allSpaces.Add(space.Name())
		}
	}
	if cache.allSpaces.IsEmpty() {
		return nil, errors.Errorf("no spaces defined")
	}
	logger.Tracef("using cached spaces: %v", cache.allSpaces.SortedValues())

	if !cache.allSpaces.Contains(tag.Id()) {
		return nil, errors.NotFoundf("space %q", tag.Id()) // " not found"
	}
	return &tag, nil
}

// cacheZones populates the allZones and availableZones cache, if it's
// empty.
func (cache *addSubnetsCache) cacheZones() error {
	if cache.allZones != nil {
		// Already cached.
		logger.Tracef("using cached zones: %v", cache.allZones.SortedValues())
		return nil
	}

	allZones, err := cache.api.AllZones()
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
	logger.Debugf(
		"%d known and %d available zones cached: %v",
		cache.allZones.Size(), cache.availableZones.Size(), cache.allZones.SortedValues(),
	)
	if cache.allZones.IsEmpty() {
		cache.allZones = nil
		// Cached an empty list.
		return errors.Errorf("no zones defined")
	}
	return nil
}

// validateZones ensures givenZones are valid. When providerZones are
// also set, givenZones must be a subset of them or match exactly.
// With non-empty providerZones and empty givenZones, it returns the
// providerZones (i.e. trusts the provider to know better). When no
// providerZones and only givenZones are set, only then the cache is
// used to validate givenZones.
func (cache *addSubnetsCache) validateZones(providerZones, givenZones []string) ([]string, error) {
	givenSet := set.NewStrings(givenZones...)
	providerSet := set.NewStrings(providerZones...)

	// First check if we can validate without using the cache.
	switch {
	case providerSet.IsEmpty() && givenSet.IsEmpty():
		return nil, errors.Errorf("Zones cannot be discovered from the provider and must be set")
	case !providerSet.IsEmpty() && givenSet.IsEmpty():
		// Use provider zones when none given.
		return providerSet.SortedValues(), nil
	case !providerSet.IsEmpty() && !givenSet.IsEmpty():
		// Ensure givenZones either match providerZones or are a
		// subset of them.
		extraGiven := givenSet.Difference(providerSet)
		if !extraGiven.IsEmpty() {
			extra := `"` + strings.Join(extraGiven.SortedValues(), `", "`) + `"`
			msg := fmt.Sprintf("Zones contain zones not allowed by the provider: %s", extra)
			return nil, errors.Errorf(msg)
		}
	}

	// Otherwise we need the cache to validate.
	if err := cache.cacheZones(); err != nil {
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

// cacheSubnets tries to get and cache once all known provider
// subnets. It handles the case when subnets have duplicated CIDRs but
// distinct ProviderIds. It also handles weird edge cases, like no
// CIDR and/or ProviderId set for a subnet.
func (cache *addSubnetsCache) cacheSubnets() error {
	if cache.allSubnets != nil {
		// Already cached.
		logger.Tracef("using %d cached subnets", len(cache.allSubnets))
		return nil
	}

	netEnv, err := cache.api.networkingEnviron()
	if err != nil {
		return errors.Trace(err)
	}
	subnetInfo, err := netEnv.Subnets(instance.UnknownId, nil)
	if err != nil {
		return errors.Annotate(err, "cannot get provider subnets")
	}
	logger.Debugf("got %d subnets to cache from the provider", len(subnetInfo))

	if len(subnetInfo) > 0 {
		// Trying to avoid reallocations.
		cache.allSubnets = make([]network.SubnetInfo, 0, len(subnetInfo))
	}
	cache.providerIdsByCIDR = make(map[string]set.Strings)
	cache.subnetsByProviderId = make(map[string]*network.SubnetInfo)

	for i, _ := range subnetInfo {
		subnet := subnetInfo[i]
		cidr := subnet.CIDR
		providerId := string(subnet.ProviderId)
		logger.Debugf(
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
	logger.Debugf("%d provider subnets cached", len(cache.allSubnets))
	if len(cache.allSubnets) == 0 {
		// Cached an empty list.
		return errors.Errorf("no subnets defined")
	}
	return nil
}

// validateSubnet ensures either subnetTag or providerId is valid (not
// both), then uses the cache to validate and lookup the provider
// SubnetInfo for the subnet, if found.
func (cache *addSubnetsCache) validateSubnet(subnetTag, providerId string) (*network.SubnetInfo, error) {
	haveTag := subnetTag != ""
	haveProviderId := providerId != ""

	if !haveTag && !haveProviderId {
		return nil, errors.Errorf("either SubnetTag or SubnetProviderId is required")
	} else if haveTag && haveProviderId {
		return nil, errors.Errorf("SubnetTag and SubnetProviderId cannot be both set")
	}
	var tag names.SubnetTag
	if haveTag {
		var err error
		tag, err = names.ParseSubnetTag(subnetTag)
		if err != nil {
			return nil, errors.Annotate(err, "given SubnetTag is invalid")
		}
	}

	// Otherwise we need the cache to validate.
	if err := cache.cacheSubnets(); err != nil {
		return nil, errors.Trace(err)
	}

	if haveTag {
		providerIds, ok := cache.providerIdsByCIDR[tag.Id()]
		if !ok || providerIds.IsEmpty() {
			return nil, errors.NotFoundf("subnet with CIDR %q", tag.Id())
		}
		if providerIds.Size() > 1 {
			ids := `"` + strings.Join(providerIds.SortedValues(), `", "`) + `"`
			return nil, errors.Errorf(
				"multiple subnets with CIDR %q: retry using ProviderId from: %s",
				tag.Id(), ids,
			)
		}
		// A single CIDR matched.
		providerId = providerIds.Values()[0]
	}

	info, ok := cache.subnetsByProviderId[providerId]
	if !ok || info == nil {
		return nil, errors.NotFoundf(
			"subnet with CIDR %q and ProviderId %q",
			tag.Id(), providerId,
		)
	}
	// Do last-call validation.
	if !names.IsValidSubnet(info.CIDR) {
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
// (initialized on first use), then adds it to the backing store, if
// successful.
func (api *subnetsAPI) addOneSubnet(args params.AddSubnetParams, cache *addSubnetsCache) error {
	subnetInfo, err := cache.validateSubnet(args.SubnetTag, args.SubnetProviderId)
	if err != nil {
		return errors.Trace(err)
	}
	spaceTag, err := cache.validateSpace(args.SpaceTag)
	if err != nil {
		return errors.Trace(err)
	}
	zones, err := cache.validateZones(subnetInfo.AvailabilityZones, args.Zones)
	if err != nil {
		return errors.Trace(err)
	}

	// Try adding the subnet.
	backingInfo := common.BackingSubnetInfo{
		ProviderId:        string(subnetInfo.ProviderId),
		CIDR:              subnetInfo.CIDR,
		VLANTag:           subnetInfo.VLANTag,
		AvailabilityZones: zones,
		SpaceName:         spaceTag.Id(),
	}
	if subnetInfo.AllocatableIPLow != nil {
		backingInfo.AllocatableIPLow = subnetInfo.AllocatableIPLow.String()
	}
	if subnetInfo.AllocatableIPHigh != nil {
		backingInfo.AllocatableIPHigh = subnetInfo.AllocatableIPHigh.String()
	}
	if _, err := api.backing.AddSubnet(backingInfo); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// AddSubnets is defined on the API interface.
func (api *subnetsAPI) AddSubnets(args params.AddSubnetsParams) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Subnets)),
	}

	if len(args.Subnets) == 0 {
		return results, nil
	}

	cache := newAddSubnetsCache(api)
	for i, arg := range args.Subnets {
		err := api.addOneSubnet(arg, cache)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
		}
	}
	return results, nil
}

// ListSubnets lists all the available subnets or only those matching
// all given optional filters.
func (api *subnetsAPI) ListSubnets(args params.SubnetsFilters) (results params.ListSubnetsResults, err error) {
	subnets, err := api.backing.AllSubnets()
	if err != nil {
		return results, errors.Trace(err)
	}

	var spaceFilter string
	if args.SpaceTag != "" {
		tag, err := names.ParseSpaceTag(args.SpaceTag)
		if err != nil {
			return results, errors.Trace(err)
		}
		spaceFilter = tag.Id()
	}
	zoneFilter := args.Zone

	for _, subnet := range subnets {
		if spaceFilter != "" && subnet.SpaceName() != spaceFilter {
			logger.Tracef(
				"filtering subnet %q from space %q not matching filter %q",
				subnet.CIDR(), subnet.SpaceName(), spaceFilter,
			)
			continue
		}
		zoneSet := set.NewStrings(subnet.AvailabilityZones()...)
		if zoneFilter != "" && !zoneSet.IsEmpty() && !zoneSet.Contains(zoneFilter) {
			logger.Tracef(
				"filtering subnet %q with zones %v not matching filter %q",
				subnet.CIDR(), subnet.AvailabilityZones(), zoneFilter,
			)
			continue
		}
		result := params.Subnet{
			CIDR:       subnet.CIDR(),
			ProviderId: subnet.ProviderId(),
			VLANTag:    subnet.VLANTag(),
			Life:       subnet.Life(),
			SpaceTag:   names.NewSpaceTag(subnet.SpaceName()).String(),
			Zones:      subnet.AvailabilityZones(),
			Status:     subnet.Status(),
		}
		results.Results = append(results.Results, result)
	}
	return results, nil
}
