// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	providercommon "github.com/juju/juju/provider/common"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.subnets")

func init() {
	common.RegisterStandardFacade("Subnets", 1, NewAPI)
}

// subnetsAPI implements the networkingcommon.SubnetsAPI interface.
type subnetsAPI struct {
	backing    networkingcommon.NetworkBacking
	resources  *common.Resources
	authorizer common.Authorizer
}

// NewAPI creates a new Subnets API server-side facade with a
// state.State backing.
func NewAPI(st *state.State, res *common.Resources, auth common.Authorizer) (networkingcommon.SubnetsAPI, error) {
	return newAPIWithBacking(networkingcommon.NewStateShim(st), res, auth)
}

// newAPIWithBacking creates a new server-side Subnets API facade with
// a common.NetworkBacking
func newAPIWithBacking(backing networkingcommon.NetworkBacking, resources *common.Resources, authorizer common.Authorizer) (networkingcommon.SubnetsAPI, error) {
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

// AddSubnets is defined on the API interface.
func (api *subnetsAPI) AddSubnets(args params.AddSubnetsParams) (params.ErrorResults, error) {
	return networkingcommon.AddSubnets(api, args)
}

// ListSubnets lists all the available subnets or only those matching
// all given optional filters.
func (api *subnetsAPI) ListSubnets(args params.SubnetsFilters) (results params.ListSubnetsResults, err error) {
	return networkingcommon.ListSubnets(api, args)
}

// Backing is defined on the API interface.
func (api *subnetsAPI) Backing() networkingcommon.NetworkBacking {
	return api.backing
}
