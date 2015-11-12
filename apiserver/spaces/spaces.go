// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.spaces")

func init() {
	common.RegisterStandardFacade("Spaces", 1, NewAPI)
}

// API defines the methods the Spaces API facade implements.
type API interface {
	CreateSpaces(params.CreateSpacesParams) (params.ErrorResults, error)
	ListSpaces() (params.ListSpacesResults, error)
}

// Backing defines the state methods this facede needs, so they can be
// mocked for testing.
type Backing interface {
	// EnvironConfig returns the configuration of the environment.
	EnvironConfig() (*config.Config, error)

	// AddSpace creates a space.
	AddSpace(name string, subnetIds []string, public bool) error

	// AllSpaces returns all known Juju network spaces.
	AllSpaces() ([]common.BackingSpace, error)
}

// spacesAPI implements the API interface.
type spacesAPI struct {
	backing    Backing
	resources  *common.Resources
	authorizer common.Authorizer
}

// NewAPI creates a new Space API server-side facade with a
// state.State backing.
func NewAPI(st *state.State, res *common.Resources, auth common.Authorizer) (API, error) {
	return newAPIWithBacking(&stateShim{st: st}, res, auth)
}

// newAPIWithBacking creates a new server-side Spaces API facade with
// the given Backing.
func newAPIWithBacking(backing Backing, resources *common.Resources, authorizer common.Authorizer) (API, error) {
	// Only clients can access the Spaces facade.
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}
	return &spacesAPI{
		backing:    backing,
		resources:  resources,
		authorizer: authorizer,
	}, nil
}

// CreateSpaces creates a new Juju network space, associating the
// specified subnets with it (optional; can be empty).
func (api *spacesAPI) CreateSpaces(args params.CreateSpacesParams) (results params.ErrorResults, err error) {
	err = api.supportsSpaces()
	if err != nil {
		return results, common.ServerError(errors.Trace(err))
	}

	results.Results = make([]params.ErrorResult, len(args.Spaces))

	for i, space := range args.Spaces {
		err := api.createOneSpace(space)
		if err == nil {
			continue
		}
		results.Results[i].Error = common.ServerError(errors.Trace(err))
	}

	return results, nil
}

func (api *spacesAPI) createOneSpace(args params.CreateSpaceParams) error {
	// Validate the args, assemble information for api.backing.AddSpaces
	var subnets []string

	spaceTag, err := names.ParseSpaceTag(args.SpaceTag)
	if err != nil {
		return errors.Trace(err)
	}

	for _, tag := range args.SubnetTags {
		subnetTag, err := names.ParseSubnetTag(tag)
		if err != nil {
			return errors.Trace(err)
		}
		subnets = append(subnets, subnetTag.Id())
	}

	// Add the validated space
	err = api.backing.AddSpace(spaceTag.Id(), subnets, args.Public)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func backingSubnetToParamsSubnet(subnet common.BackingSubnet) params.Subnet {
	cidr := subnet.CIDR()
	vlantag := subnet.VLANTag()
	providerid := subnet.ProviderId()
	zones := subnet.AvailabilityZones()
	status := subnet.Status()
	var spaceTag names.SpaceTag
	if subnet.SpaceName() != "" {
		spaceTag = names.NewSpaceTag(subnet.SpaceName())
	}

	return params.Subnet{
		CIDR:       cidr,
		VLANTag:    vlantag,
		ProviderId: providerid,
		Zones:      zones,
		Status:     status,
		SpaceTag:   spaceTag.String(),
		Life:       subnet.Life(),
	}
}

// ListSpaces lists all the available spaces and their associated subnets.
func (api *spacesAPI) ListSpaces() (results params.ListSpacesResults, err error) {
	err = api.supportsSpaces()
	if err != nil {
		return results, common.ServerError(errors.Trace(err))
	}

	spaces, err := api.backing.AllSpaces()
	if err != nil {
		return results, errors.Trace(err)
	}

	results.Results = make([]params.Space, len(spaces))
	for i, space := range spaces {
		result := params.Space{}
		result.Name = space.Name()

		subnets, err := space.Subnets()
		if err != nil {
			err = errors.Annotatef(err, "fetching subnets")
			result.Error = common.ServerError(err)
			results.Results[i] = result
			continue
		}

		result.Subnets = make([]params.Subnet, len(subnets))
		for i, subnet := range subnets {
			result.Subnets[i] = backingSubnetToParamsSubnet(subnet)
		}
		results.Results[i] = result
	}
	return results, nil
}

// supportsSpaces checks if the environment implements NetworkingEnviron
// and also if it supports spaces.
func (api *spacesAPI) supportsSpaces() error {
	config, err := api.backing.EnvironConfig()
	if err != nil {
		return errors.Annotate(err, "getting environment config")
	}
	env, err := environs.New(config)
	if err != nil {
		return errors.Annotate(err, "validating environment config")
	}
	netEnv, ok := environs.SupportsNetworking(env)
	if !ok {
		return errors.NotSupportedf("networking")
	}
	ok, err = netEnv.SupportsSpaces()
	if !ok {
		return errors.NotSupportedf("spaces")
	}
	return err
}
