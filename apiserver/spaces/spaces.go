// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
)

var logger = loggo.GetLogger("juju.apiserver.spaces")

func init() {
	// TODO(dimitern): Uncomment once *state.State implements Backing.
	// common.RegisterStandardFacade("Spaces", 1, NewAPI)
}

// API defines the methods the Spaces API facade implements.
type API interface {
	CreateSpaces(params.AddSpacesParams) (params.ErrorResults, error)
}

// spacesAPI implements the API interface.
type spacesAPI struct {
	backing    common.NetworkBacking
	resources  *common.Resources
	authorizer common.Authorizer
}

var _ API = (*spacesAPI)(nil)

// NewAPI creates a new server-side Spaces API facade.
func NewAPI(backing common.NetworkBacking, resources *common.Resources, authorizer common.Authorizer) (API, error) {
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

// AddSpaces creates a new Juju network space, associating the
// specified subnets with it (optional; can be empty).
func (api *spacesAPI) CreateSpaces(args params.AddSpacesParams) (params.ErrorResults, error) {
	results := params.ErrorResults{}

	for _, space := range args.Spaces {
		err := api.createOneSpace(space)
		errorResult := params.ErrorResult{}
		if err != nil {
			errors.Trace(err)
			errorResult.Error = common.ServerError(err)
		}

		results.Results = append(results.Results, errorResult)
	}

	return results, nil
}

func (api *spacesAPI) createOneSpace(args params.AddSpaceParams) error {
	if len(args.SubnetTags) == 0 {
		return errors.NotValidf("calling CreateSpaces with zero subnets is") // ... not valid.
	}

	// Validate the args, assemble information for api.backing.AddSpaces
	var subnets []string

	spaceTag, err := names.ParseSpaceTag(args.SpaceTag)
	if err != nil {
		return errors.Annotate(err, "given SpaceTag is invalid")
	}

	for _, tag := range args.SubnetTags {
		if subnetTag, err := names.ParseSubnetTag(tag); err != nil {
			return errors.Annotate(err, "given SubnetTag is invalid")
		} else {
			subnets = append(subnets, subnetTag.Id())
		}
	}

	// Add the validated space
	if err := api.backing.AddSpace(spaceTag.Id(), subnets, args.Public); err != nil {
		return errors.Annotate(err, "cannot create space")
	}
	return nil
}
