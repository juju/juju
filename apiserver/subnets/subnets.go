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
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	providercommon "github.com/juju/juju/provider/common"
)

var logger = loggo.GetLogger("juju.apiserver.subnets")

func init() {
	// TODO(dimitern): Uncomment once *state.State implements Backing.
	//common.RegisterStandardFacade("Subnets", 1, NewAPI)
}

// BackingSpace defines the methods supported by a Space entity stored
// persistently.
//
// TODO(dimitern): Once *state.Space is implemented, ensure it has
// those methods, move the interface somewhere common and rename it as
// needed, and change Backing.AllSpaces() to return that.
type BackingSpace interface {
	// Name returns the space name.
	Name() string
}

// BackingSubnet defines the methods supported by a Subnet entity
// stored persistently.
//
// TODO(dimitern): Once the state backing is implemented, remove this
// and just use *state.Subnet.
type BackingSubnet interface {
	// no methods needed yet.
}

// BackingSubnetInfo describes a single subnet to be added in the
// backing store.
//
// TODO(dimitern): Replace state.SubnetInfo with this and remove
// BackingSubnetInfo, once the rest of state backing methods and the
// following pre-reqs are done:
// * subnetDoc.AvailabilityZone becomes subnetDoc.AvailabilityZones,
//   adding an upgrade step to migrate existing non empty zones on
//   subnet docs. Also change state.Subnet.AvailabilityZone to
// * add subnetDoc.SpaceName - no upgrade step needed, as it will only
//   be used for new space-aware subnets.
// * ensure EC2 and MAAS providers accept empty IDs as Subnets() args
//   and return all subnets, including the AvailabilityZones (for EC2;
//   empty for MAAS as zones are orthogonal to networks).
type BackingSubnetInfo struct {
	// ProviderId is a provider-specific network id. This may be empty.
	ProviderId string

	// CIDR of the network, in 123.45.67.89/24 format.
	CIDR string

	// VLANTag needs to be between 1 and 4094 for VLANs and 0 for normal
	// networks. It's defined by IEEE 802.1Q standard.
	VLANTag int

	// AllocatableIPHigh and Low describe the allocatable portion of the
	// subnet. The remainder, if any, is reserved by the provider.
	// Either both of these must be set or neither, if they're empty it
	// means that none of the subnet is allocatable. If present they must
	// be valid IP addresses within the subnet CIDR.
	AllocatableIPHigh string
	AllocatableIPLow  string

	// AvailabilityZones describes which availability zone(s) this
	// subnet is in. It can be empty if the provider does not support
	// availability zones.
	AvailabilityZones []string

	// SpaceName holds the juju network space this subnet is
	// associated with. Can be empty if not supported.
	SpaceName string
}

// Backing defines the methods needed by the API facade to store and
// retrieve information from the underlying persistency layer (state
// DB).
type Backing interface {
	// EnvironConfig returns the current environment config.
	EnvironConfig() (*config.Config, error)

	// AvailabilityZones returns all cached availability zones (i.e.
	// not from the provider, but in state).
	AvailabilityZones() ([]providercommon.AvailabilityZone, error)

	// SetAvailabilityZones replaces the cached list of availability
	// zones with the given zones.
	SetAvailabilityZones(zones []providercommon.AvailabilityZone) error

	// AllSpaces returns all known Juju network spaces.
	AllSpaces() ([]BackingSpace, error)

	// AddSubnet creates a backing subnet for an existing subnet.
	AddSubnet(subnetInfo BackingSubnetInfo) (BackingSubnet, error)
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
}

// internalAPI implements the API interface.
type internalAPI struct {
	backing    Backing
	resources  *common.Resources
	authorizer common.Authorizer
}

var _ API = (*internalAPI)(nil)

// NewAPI creates a new server-side Subnets API facade.
func NewAPI(backing Backing, resources *common.Resources, authorizer common.Authorizer) (API, error) {
	// Only clients can access the Subnets facade.
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}
	return &internalAPI{
		backing:    backing,
		resources:  resources,
		authorizer: authorizer,
	}, nil
}

// AllZones is defined on the API interface.
func (a *internalAPI) AllZones() (params.ZoneResults, error) {
	var results params.ZoneResults

	zonesAsString := func(zones []providercommon.AvailabilityZone) string {
		results := make([]string, len(zones))
		for i, zone := range zones {
			results[i] = zone.Name()
		}
		return `"` + strings.Join(results, `", "`) + `"`
	}

	// Try fetching cached zones first.
	zones, err := a.backing.AvailabilityZones()
	if err != nil {
		return results, errors.Trace(err)
	}

	if len(zones) == 0 {
		// This is likely the first time we're called.
		// Fetch all zones from the provider and update.
		zones, err = a.updateZones()
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
func (a *internalAPI) AllSpaces() (params.SpaceResults, error) {
	var results params.SpaceResults

	spaces, err := a.backing.AllSpaces()
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
func (a *internalAPI) zonedEnviron() (providercommon.ZonedEnviron, error) {
	envConfig, err := a.backing.EnvironConfig()
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
func (a *internalAPI) networkingEnviron() (environs.NetworkingEnviron, error) {
	envConfig, err := a.backing.EnvironConfig()
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
func (a *internalAPI) updateZones() ([]providercommon.AvailabilityZone, error) {
	zoned, err := a.zonedEnviron()
	if err != nil {
		return nil, errors.Trace(err)
	}
	zones, err := zoned.AvailabilityZones()
	if err != nil {
		return nil, errors.Trace(err)
	}

	if err := a.backing.SetAvailabilityZones(zones); err != nil {
		return nil, errors.Trace(err)
	}
	return zones, nil
}

type addSubnetsCache struct {
	api            *internalAPI
	allSpaces      set.Strings
	allZones       set.Strings
	availableZones set.Strings
	allSubnets     []network.SubnetInfo
	// providerIdsByCIDR maps possibly duplicated CIDRs to one or more ids.
	providerIdsByCIDR   map[string]set.Strings
	subnetsByProviderId map[string]*network.SubnetInfo
}

func initAddSubnetsCache(api *internalAPI) *addSubnetsCache {
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

func (a *addSubnetsCache) validateSpace(spaceTag string) (*names.SpaceTag, error) {
	if spaceTag == "" {
		return nil, errors.Errorf("SpaceTag is required")
	}
	tag, err := names.ParseSpaceTag(spaceTag)
	if err != nil {
		return nil, errors.Annotate(err, "given SpaceTag is invalid")
	}

	// Otherwise we need the cache to validate.
	if a.allSpaces == nil {
		// Not yet cached.
		logger.Debugf("caching known spaces")

		allSpaces, err := a.api.backing.AllSpaces()
		if err != nil {
			return nil, errors.Annotate(err, "cannot validate given SpaceTag")
		}
		a.allSpaces = set.NewStrings()
		for _, space := range allSpaces {
			if a.allSpaces.Contains(space.Name()) {
				logger.Warningf("ignoring duplicated space %q", space.Name())
				continue
			}
			a.allSpaces.Add(space.Name())
		}
	}
	if a.allSpaces.IsEmpty() {
		return nil, errors.Errorf("no spaces defined")
	}
	logger.Tracef("using cached spaces: %v", a.allSpaces.SortedValues())

	if !a.allSpaces.Contains(tag.Id()) {
		return nil, errors.NotFoundf("given SpaceTag %q", tag.String()) // " not found"
	}
	return &tag, nil
}

func (a *addSubnetsCache) cacheZones() error {
	if a.allZones != nil {
		// Already cached.
		logger.Tracef("using cached zones: %v", a.allZones.SortedValues())
		return nil
	}

	allZones, err := a.api.AllZones()
	if err != nil {
		return errors.Annotate(err, "given Zones cannot be validated")
	}
	a.allZones = set.NewStrings()
	a.availableZones = set.NewStrings()
	for _, zone := range allZones.Results {
		// AllZones() does not use the Error result field, so no
		// need to check it here.
		if a.allZones.Contains(zone.Name) {
			logger.Warningf("ignoring duplicated zone %q", zone.Name)
			continue
		}

		if zone.Available {
			a.availableZones.Add(zone.Name)
		}
		a.allZones.Add(zone.Name)
	}
	logger.Debugf(
		"%d known and %d available zones cached: %v",
		a.allZones.Size(), a.availableZones.Size(), a.allZones.SortedValues(),
	)
	if a.allZones.IsEmpty() {
		a.allZones = nil
		// Cached an empty list.
		return errors.Errorf("no zones defined")
	}
	return nil
}

func (a *addSubnetsCache) validateZones(providerZones, givenZones []string) ([]string, error) {
	haveProviderZones := len(providerZones) > 0
	haveGivenZones := len(givenZones) > 0
	givenSet := set.NewStrings(givenZones...)

	// First check if we can validate without using the cache.
	if !haveProviderZones && !haveGivenZones {
		return nil, errors.Errorf("Zones cannot be discovered from the provider and must be set")
	} else if haveProviderZones {
		providerSet := set.NewStrings(providerZones...)
		if !haveGivenZones {
			// Use provider zones when none given.
			return providerSet.SortedValues(), nil
		}

		extraGiven := givenSet.Difference(providerSet)
		if !extraGiven.IsEmpty() {
			extra := `"` + strings.Join(extraGiven.SortedValues(), `", "`) + `"`
			msg := fmt.Sprintf("Zones contain zones not allowed by the provider: %s", extra)
			return nil, errors.Errorf(msg)
		}
	}

	// Otherwise we need the cache to validate.
	if err := a.cacheZones(); err != nil {
		return nil, errors.Trace(err)
	}

	diffAvailable := givenSet.Difference(a.availableZones)
	diffAll := givenSet.Difference(a.allZones)

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

func (a *addSubnetsCache) cacheSubnets() error {
	if a.allSubnets != nil {
		// Already cached.
		logger.Tracef("using %d cached subnets", len(a.allSubnets))
		return nil
	}

	netEnv, err := a.api.networkingEnviron()
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
		a.allSubnets = make([]network.SubnetInfo, 0, len(subnetInfo))
	}
	a.providerIdsByCIDR = make(map[string]set.Strings)
	a.subnetsByProviderId = make(map[string]*network.SubnetInfo)

	for i, _ := range subnetInfo {
		subnet := subnetInfo[i]
		cidr := subnet.CIDR
		providerId := string(subnet.ProviderId)
		logger.Debugf("caching subnet with CIDR %q and ProviderId %q", cidr, providerId)

		if providerId == "" && cidr == "" {
			logger.Warningf("found subnet with empty CIDR and ProviderId")
			// But we still save it for lookups, which will probably fail anyway.
		} else if providerId == "" {
			logger.Warningf("found subnet with CIDR %q and empty ProviderId", cidr)
			// But we still save it for lookups.
		} else {
			_, ok := a.subnetsByProviderId[providerId]
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
		a.subnetsByProviderId[providerId] = &subnet

		if ids, ok := a.providerIdsByCIDR[cidr]; !ok {
			a.providerIdsByCIDR[cidr] = set.NewStrings(providerId)
		} else {
			ids.Add(providerId)
			logger.Debugf(
				"duplicated subnet CIDR %q; collected ProviderIds so far: %v",
				cidr, ids.SortedValues(),
			)
			a.providerIdsByCIDR[cidr] = ids
		}

		a.allSubnets = append(a.allSubnets, subnet)
	}
	logger.Debugf("%d provider subnets cached", len(a.allSubnets))
	if len(a.allSubnets) == 0 {
		// Cached an empty list.
		return errors.Errorf("no subnets defined")
	}
	return nil
}

func (a *addSubnetsCache) validateSubnet(subnetTag, providerId string) (*network.SubnetInfo, error) {
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
	if err := a.cacheSubnets(); err != nil {
		return nil, errors.Trace(err)
	}

	if haveTag {
		providerIds, ok := a.providerIdsByCIDR[tag.Id()]
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

	info, ok := a.subnetsByProviderId[providerId]
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

func (a *internalAPI) addOneSubnet(args params.AddSubnetParams, cache *addSubnetsCache) error {
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
	backingInfo := BackingSubnetInfo{
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
	if _, err := a.backing.AddSubnet(backingInfo); err != nil {
		return errors.Annotate(err, "cannot add subnet")
	}
	return nil
}

// AddSubnets is defined on the API interface.
func (a *internalAPI) AddSubnets(args params.AddSubnetsParams) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Subnets)),
	}

	if len(args.Subnets) == 0 {
		return results, nil
	}

	cache := initAddSubnetsCache(a)
	for i, arg := range args.Subnets {
		err := a.addOneSubnet(arg, cache)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
		}
	}
	return results, nil
}
