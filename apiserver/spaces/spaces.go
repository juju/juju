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

type BackingSpaceInfo struct {
	Name    string
	Subnets []string
}

// API defines the methods the Spaces API facade implements.
type API interface {
	CreateSpace(params.CreateSpaceParams) params.ErrorResult
}

// Backing defines the methods needed by the API facade to store and
// retrieve information from the underlying persistency layer (state
// DB).
type SubnetBacking interface {
	common.NetworkBacking

	// AllSpaces returns all known Juju network spaces.
	CreateSpace(BackingSpaceInfo) error
}

// spacesAPI implements the API interface.
type spacesAPI struct {
	backing    SubnetBacking
	resources  *common.Resources
	authorizer common.Authorizer
}

var _ API = (*spacesAPI)(nil)

// NewAPI creates a new server-side Subnets API facade.
func NewAPI(backing SubnetBacking, resources *common.Resources, authorizer common.Authorizer) (API, error) {
	// Only clients can access the Subnets facade.
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}
	return &spacesAPI{
		backing:    backing,
		resources:  resources,
		authorizer: authorizer,
	}, nil
}

// CreateSpace creates a new Juju network space, associating the
// specified subnets with it (optional; can be empty).
func (api *spacesAPI) CreateSpace(args params.CreateSpaceParams) (result params.ErrorResult) {
	err := api.createSpace(args)
	result.Error = common.ServerError(err)
	return
}

// createSpace does the work of CreateSpace
func (api *spacesAPI) createSpace(args params.CreateSpaceParams) error {
	// If we don't receive any subnets to create the space out of, ignore the call.
	if len(args.SubnetTags) == 0 {
		return nil
	}

	// Validate the args, assemble information for api.backing.CreateSpace
	var newSpace BackingSpaceInfo

	spaceTag, err := names.ParseSpaceTag(args.SpaceTag)
	if err != nil {
		return errors.Annotate(err, "given SpaceTag is invalid")
	}
	newSpace.Name = spaceTag.Id()

	for _, tag := range args.SubnetTags {
		if subnetTag, err := names.ParseSubnetTag(tag); err != nil {
			return errors.Annotate(err, "given SubnetTag is invalid")
		} else {
			newSpace.Subnets = append(newSpace.Subnets, subnetTag.Id())
		}
	}

	// Add the validated subnet
	if err := api.backing.CreateSpace(newSpace); err != nil {
		return errors.Annotate(err, "cannot add space")
	}
	return nil
}
