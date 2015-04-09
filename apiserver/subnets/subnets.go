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
	providercommon "github.com/juju/juju/provider/common"
)

var logger = loggo.GetLogger("juju.apiserver.subnets")

func init() {
	// TODO(dimitern): Uncomment once *state.State implements Backing.
	//common.RegisterStandardFacade("Subnets", 1, NewAPI)
}

// BackingSpace defines the methods supported by a Space entity stored
// persistenty.
//
// TODO(dimitern): Once *state.Space is implemented, ensure it has
// those methods, move the interface in state/ and rename it as
// needed, and change Backing.AllSpaces() to return that.
type BackingSpace interface {
	// Name returns the space name.
	Name() string
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
}

// API defines the methods the Subnets API facade implements.
type API interface {
	// AllZones returns all availability zones known to Juju. If a
	// zone is unusable, unavailable, or deprecated the Available
	// field will be false.
	AllZones() (params.ZoneResults, error)

	// AllSpaces returns the tags of all network spaces known to Juju.
	AllSpaces() (params.SpaceResults, error)
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
		results.Results[i].Tag = tag.Id()
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
