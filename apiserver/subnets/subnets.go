// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

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
		logger.Debugf("updated the list of known zones from the environment: %v", zones)
	} else {
		logger.Debugf("using cached list of known zones: %v", zones)
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
		return nil, errors.Annotate(err, "getting environment")
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
		return nil, errors.Annotate(err, "getting environment")
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

func (a *internalAPI) addOneSubnet(args params.AddSubnetParams, cachedInfo *[]network.SubnetInfo) error {
	// Validate required arguments.
	if args.SubnetTag == "" && args.SubnetProviderId == "" {
		return errors.Errorf("either SubnetTag or SubnetProviderId is required")
	} else if args.SubnetTag != "" && args.SubnetProviderId != "" {
		return errors.Errorf("SubnetTag or SubnetProviderId cannot be both set")
	}
	if args.SpaceTag == "" {
		return errors.Errorf("SpaceTag is required")
	}
	spaceTag, err := names.ParseSpaceTag(args.SpaceTag)
	if err != nil {
		return errors.Annotate(err, "invalid space tag given")
	}

	// Get all subnets (from cache or the provider).
	var subnetInfo []network.SubnetInfo
	if cachedInfo == nil || len(*cachedInfo) == 0 {
		netEnv, err := a.networkingEnviron()
		if err != nil {
			return errors.Trace(err)
		}
		subnetInfo, err = netEnv.Subnets(instance.UnknownId, nil)
		if err != nil {
			return errors.Annotate(err, "cannot get provider subnets")
		}
		// Only cache them if requested (cachedInfo != nil).
		if cachedInfo != nil {
			*cachedInfo = make([]network.SubnetInfo, len(subnetInfo))
			copy(*cachedInfo, subnetInfo)
		}
	} else if cachedInfo != nil && len(*cachedInfo) > 0 {
		// Use the cache instead.
		subnetInfo = make([]network.SubnetInfo, len(*cachedInfo))
		copy(subnetInfo, *cachedInfo)
	}
	if len(subnetInfo) == 0 {
		return errors.Errorf("cannot find any subnets")
	}

	// Use the tag if specified, otherwise use the provider ID.
	var subnetTag names.SubnetTag
	var providerId network.Id
	if args.SubnetTag != "" {
		subnetTag, err = names.ParseSubnetTag(args.SubnetTag)
		if err != nil {
			return errors.Annotate(err, "invalid subnet tag given")
		}
	} else {
		providerId = network.Id(args.SubnetProviderId)
	}

	// Find a matching subnet info - by tag or provider ID.
	var sub network.SubnetInfo
	for _, info := range subnetInfo {
		if (info.CIDR == subnetTag.Id() && info.CIDR != "") ||
			(info.ProviderId == providerId && info.ProviderId != "") {
			sub = info
			break
		}
	}
	// Return nicer error when we can't find it.
	if sub.CIDR == "" {
		if args.SubnetProviderId != "" {
			return errors.NotFoundf("subnet with ProviderId %q", args.SubnetProviderId)
		}
		return errors.NotFoundf("subnet %q", subnetTag.Id())
	}

	// At this point zones must be specified if cannot be discovered.
	if len(sub.AvailabilityZones) == 0 {
		return errors.Errorf("Zones cannot be discovered from the provider and must be set")
	}

	// Try adding the subnet.
	backingInfo := BackingSubnetInfo{
		ProviderId:        string(sub.ProviderId),
		CIDR:              sub.CIDR,
		AvailabilityZones: sub.AvailabilityZones,
		SpaceName:         spaceTag.Id(),
	}
	if sub.AllocatableIPLow != nil {
		backingInfo.AllocatableIPLow = sub.AllocatableIPLow.String()
	}
	if sub.AllocatableIPHigh != nil {
		backingInfo.AllocatableIPHigh = sub.AllocatableIPHigh.String()
	}
	if _, err := a.backing.AddSubnet(backingInfo); err != nil {
		return errors.Annotate(err, "cannot add subnet")
	}
	return nil
}

// AddSubnets is defined on the API interface.
func (a *internalAPI) AddSubnets(args params.AddSubnetsParams) (params.ErrorResults, error) {
	results := params.ErrorResults{Results: make([]params.ErrorResult, len(args.Subnets))}

	if len(args.Subnets) == 0 {
		return results, nil
	}

	var cache []network.SubnetInfo
	for i, arg := range args.Subnets {
		err := a.addOneSubnet(arg, &cache)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
		}
	}
	return results, nil
}
