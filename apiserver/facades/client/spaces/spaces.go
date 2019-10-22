// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
)

// BlockChecker defines the block-checking functionality required by
// the spaces facade. This is implemented by apiserver/common.BlockChecker.
type BlockChecker interface {
	ChangeAllowed() error
	RemoveAllowed() error
}

// Backend contains the state methods used in this package.
type Backing interface {
	environs.EnvironConfigGetter

	// ModelTag returns the tag of this model.
	ModelTag() names.ModelTag

	// SubnetByCIDR returns a subnet based on the input CIDR.
	SubnetByCIDR(cidr string) (networkingcommon.BackingSubnet, error)

	// AddSpace creates a space.
	AddSpace(Name string, ProviderId network.Id, Subnets []string, Public bool) error

	// AllSpaces returns all known Juju network spaces.
	AllSpaces() ([]networkingcommon.BackingSpace, error)

	// ReloadSpaces loads spaces from backing environ.
	ReloadSpaces(environ environs.BootstrapEnviron) error
}

// APIv2 provides the spaces API facade for versions < 3.
type APIv2 struct {
	*APIv3
}

// APIv3 provides the spaces API facade for version 3.
type APIv3 struct {
	*APIv4
}

// APIv4 provides the spaces API facade for version 4.
type APIv4 struct {
	*API
}

// API provides the spaces API facade for version 5.
type API struct {
	backing    Backing
	resources  facade.Resources
	authorizer facade.Authorizer
	context    context.ProviderCallContext

	check BlockChecker
}

// NewAPIv2 is a wrapper that creates a V2 spaces API.
func NewAPIv2(st *state.State, res facade.Resources, auth facade.Authorizer) (*APIv2, error) {
	api, err := NewAPIv3(st, res, auth)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv2{api}, nil
}

// NewAPIv3 is a wrapper that creates a V3 spaces API.
func NewAPIv3(st *state.State, res facade.Resources, auth facade.Authorizer) (*APIv3, error) {
	api, err := NewAPIv4(st, res, auth)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv3{api}, nil
}

// NewAPIv4 is a wrapper that creates a V4 spaces API.
func NewAPIv4(st *state.State, res facade.Resources, auth facade.Authorizer) (*APIv4, error) {
	api, err := NewAPI(st, res, auth)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv4{api}, nil
}

// NewAPI creates a new Space API server-side facade with a
// state.State backing.
func NewAPI(st *state.State, res facade.Resources, auth facade.Authorizer) (*API, error) {
	stateShim, err := NewStateShim(st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newAPIWithBacking(stateShim, common.NewBlockChecker(st), state.CallContext(st), res, auth)
}

// newAPIWithBacking creates a new server-side Spaces API facade with
// the given Backing.
func newAPIWithBacking(
	backing Backing,
	check BlockChecker,
	ctx context.ProviderCallContext,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*API, error) {
	// Only clients can access the Spaces facade.
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}
	return &API{
		backing:    backing,
		resources:  resources,
		authorizer: authorizer,
		context:    ctx,
		check:      check,
	}, nil
}

// CreateSpaces creates a new Juju network space, associating the
// specified subnets with it (optional; can be empty).
func (api *API) CreateSpaces(args params.CreateSpacesParams) (results params.ErrorResults, err error) {
	isAdmin, err := api.authorizer.HasPermission(permission.AdminAccess, api.backing.ModelTag())
	if err != nil && !errors.IsNotFound(err) {
		return results, errors.Trace(err)
	}
	if !isAdmin {
		return results, common.ServerError(common.ErrPerm)
	}
	if err := api.check.ChangeAllowed(); err != nil {
		return results, errors.Trace(err)
	}
	if err = api.checkSupportsSpaces(); err != nil {
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

// CreateSpaces creates a new Juju network space, associating the
// specified subnets with it (optional; can be empty).
func (api *APIv4) CreateSpaces(args params.CreateSpacesParamsV4) (params.ErrorResults, error) {
	isAdmin, err := api.authorizer.HasPermission(permission.AdminAccess, api.backing.ModelTag())
	if err != nil && !errors.IsNotFound(err) {
		return params.ErrorResults{}, errors.Trace(err)
	}
	if !isAdmin {
		return params.ErrorResults{}, common.ServerError(common.ErrPerm)
	}
	if err := api.check.ChangeAllowed(); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	if err := api.checkSupportsSpaces(); err != nil {
		return params.ErrorResults{}, common.ServerError(errors.Trace(err))
	}

	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Spaces)),
	}

	for i, space := range args.Spaces {
		cidrs, err := convertOldSubnetTagToCIDR(space.SubnetTags)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		csParams := params.CreateSpaceParams{
			CIDRs:      cidrs,
			SpaceTag:   space.SpaceTag,
			Public:     space.Public,
			ProviderId: space.ProviderId,
		}
		err = api.createOneSpace(csParams)
		if err == nil {
			continue
		}
		results.Results[i].Error = common.ServerError(errors.Trace(err))
	}

	return results, nil
}

// createOneSpace creates one new Juju network space, associating the
// specified subnets with it (optional; can be empty).
func (api *API) createOneSpace(args params.CreateSpaceParams) error {
	// Validate the args, assemble information for api.backing.AddSpaces
	spaceTag, err := names.ParseSpaceTag(args.SpaceTag)
	if err != nil {
		return errors.Trace(err)
	}

	subnetIDs := make([]string, len(args.CIDRs))
	for i, cidr := range args.CIDRs {
		if !network.IsValidCidr(cidr) {
			return errors.New(fmt.Sprintf("%q is not a valid CIDR", cidr))
		}
		subnet, err := api.backing.SubnetByCIDR(cidr)
		if err != nil {
			return err
		}
		subnetIDs[i] = subnet.ID()
	}

	// Add the validated space.
	err = api.backing.AddSpace(spaceTag.Id(), network.Id(args.ProviderId), subnetIDs, args.Public)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func convertOldSubnetTagToCIDR(subnetTags []string) ([]string, error) {
	cidrs := make([]string, len(subnetTags))
	// in lieu of keeping names.v2 around, split the expected
	// string for the older api calls.  Format: subnet-<cidr>
	for i, tag := range subnetTags {
		split := strings.Split(tag, "-")
		if len(split) != 2 || split[0] != "subnet" {
			return nil, errors.New(fmt.Sprintf("%q is not valid SubnetTag", tag))
		}
		cidrs[i] = split[1]
	}
	return cidrs, nil
}

// ListSpaces lists all the available spaces and their associated subnets.
func (api *API) ListSpaces() (results params.ListSpacesResults, err error) {
	canRead, err := api.authorizer.HasPermission(permission.ReadAccess, api.backing.ModelTag())
	if err != nil && !errors.IsNotFound(err) {
		return results, errors.Trace(err)
	}
	if !canRead {
		return results, common.ServerError(common.ErrPerm)
	}

	err = api.checkSupportsSpaces()
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
		result.Id = space.Id()
		if space.Name() != network.DefaultSpaceName {
			result.Name = space.Name()
		}

		subnets, err := space.Subnets()
		if err != nil {
			err = errors.Annotatef(err, "fetching subnets")
			result.Error = common.ServerError(err)
			results.Results[i] = result
			continue
		}

		result.Subnets = make([]params.Subnet, len(subnets))
		for i, subnet := range subnets {
			result.Subnets[i] = networkingcommon.BackingSubnetToParamsSubnet(subnet)
		}
		results.Results[i] = result
	}
	return results, nil
}

// ReloadSpaces is not available via the V2 API.
func (u *APIv2) ReloadSpaces(_, _ struct{}) {}

// RefreshSpaces refreshes spaces from substrate
func (api *API) ReloadSpaces() error {
	canWrite, err := api.authorizer.HasPermission(permission.WriteAccess, api.backing.ModelTag())
	if err != nil && !errors.IsNotFound(err) {
		return errors.Trace(err)
	}
	if !canWrite {
		return common.ServerError(common.ErrPerm)
	}
	if err := api.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	env, err := environs.GetEnviron(api.backing, environs.New)
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(api.backing.ReloadSpaces(env))
}

// checkSupportsSpaces checks if the environment implements NetworkingEnviron
// and also if it supports spaces.
func (api *API) checkSupportsSpaces() error {
	env, err := environs.GetEnviron(api.backing, environs.New)
	if err != nil {
		return errors.Annotate(err, "getting environ")
	}
	if !environs.SupportsSpaces(api.context, env) {
		return errors.NotSupportedf("spaces")
	}
	return nil
}
