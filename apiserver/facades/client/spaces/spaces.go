// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"fmt"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	jujucontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.spaces")

// BlockChecker defines the block-checking functionality required by
// the spaces facade. This is implemented by apiserver/common.BlockChecker.
type BlockChecker interface {
	ChangeAllowed() error
	RemoveAllowed() error
}

// Address is an indirection for state.Address.
type Address interface {
	SubnetCIDR() string
}

// Machine defines the methods supported by a machine used in the space context.
type Machine interface {
	AllSpaces() (set.Strings, error)
	AllAddresses() ([]Address, error)
	Id() string
}

// Constraints defines the methods supported by constraints used in the space context.
type Constraints interface {
	ID() string
}

// ApplicationEndpointBindingsShim is a shim interface for stateless access to ApplicationEndpointBindings
type ApplicationEndpointBindingsShim struct {
	AppName  string
	Bindings map[string]string
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

	// SpaceByName returns the Juju network space given by name.
	SpaceByName(name string) (networkingcommon.BackingSpace, error)

	// ReloadSpaces loads spaces from backing environ.
	ReloadSpaces(environ environs.BootstrapEnviron) error

	// AllEndpointBindings loads all endpointBindings.
	AllEndpointBindings() ([]ApplicationEndpointBindingsShim, error)

	// AllMachines loads all machines.
	AllMachines() ([]Machine, error)

	// ApplyOperation applies a given ModelOperation to the model.
	ApplyOperation(state.ModelOperation) error

	// ControllerConfig Returns the controller config.
	ControllerConfig() (jujucontroller.Config, error)

	// ConstraintsBySpaceName  Returns constraints found by spaceName.
	ConstraintsBySpaceName(name string) ([]Constraints, error)

	// IsController returns true if this state instance has the bootstrap
	// model UUID.
	IsController() bool
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
	*APIv5
}

// APIv5 provides the spaces API facade for version 5.
type APIv5 struct {
	*API
}

// API provides the spaces API facade for version 6.
type API struct {
	backing   Backing
	resources facade.Resources
	auth      facade.Authorizer
	context   context.ProviderCallContext

	check     BlockChecker
	opFactory OpFactory
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
	api, err := NewAPIv5(st, res, auth)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv4{api}, nil
}

// NewAPIv4 is a wrapper that creates a V4 spaces API.
func NewAPIv5(st *state.State, res facade.Resources, auth facade.Authorizer) (*APIv5, error) {
	api, err := NewAPI(st, res, auth)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv5{api}, nil
}

// NewAPI creates a new Space API server-side facade with a
// state.State backing.
func NewAPI(st *state.State, res facade.Resources, auth facade.Authorizer) (*API, error) {
	stateShim, err := NewStateShim(st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newAPIWithBacking(stateShim, common.NewBlockChecker(st), state.CallContext(st), res, auth, newOpFactory(st))
}

// newAPIWithBacking creates a new server-side Spaces API facade with
// the given Backing.
func newAPIWithBacking(
	backing Backing,
	check BlockChecker,
	ctx context.ProviderCallContext,
	resources facade.Resources,
	authorizer facade.Authorizer,
	factory OpFactory,
) (*API, error) {
	// Only clients can access the Spaces facade.
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}
	return &API{
		backing:   backing,
		resources: resources,
		auth:      authorizer,
		context:   ctx,
		check:     check,
		opFactory: factory,
	}, nil
}

// CreateSpaces creates a new Juju network space, associating the
// specified subnets with it (optional; can be empty).
func (api *API) CreateSpaces(args params.CreateSpacesParams) (results params.ErrorResults, err error) {
	isAdmin, err := api.auth.HasPermission(permission.AdminAccess, api.backing.ModelTag())
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
	isAdmin, err := api.auth.HasPermission(permission.AdminAccess, api.backing.ModelTag())
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

	subnets, err := api.getValidSubnetsByCIDR(args.CIDRs)
	subnetIDs := make([]string, len(subnets))
	for i, subnet := range subnets {
		subnetIDs[i] = subnet.ID()
	}
	if err != nil {
		return errors.Trace(err)
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
	// string for the older api calls.  Format: subnet-<CIDR>
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
	canRead, err := api.auth.HasPermission(permission.ReadAccess, api.backing.ModelTag())
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
			result.Subnets[i] = networkingcommon.BackingSubnetToParamsSubnet(subnet)
		}
		results.Results[i] = result
	}
	return results, nil
}

func (api *APIv5) ShowSpace(_, _ struct{}) {}

// ListSpaces lists all the available spaces and their associated subnets.
func (api *API) ShowSpace(entities params.Entities) (params.ShowSpaceResults, error) {
	canRead, err := api.auth.HasPermission(permission.ReadAccess, api.backing.ModelTag())
	if err != nil && !errors.IsNotFound(err) {
		return params.ShowSpaceResults{}, errors.Trace(err)
	}
	if !canRead {
		return params.ShowSpaceResults{}, common.ServerError(common.ErrPerm)
	}

	err = api.checkSupportsSpaces()
	if err != nil {
		return params.ShowSpaceResults{}, common.ServerError(errors.Trace(err))
	}
	results := make([]params.ShowSpaceResult, len(entities.Entities))
	for i, entity := range entities.Entities {
		spaceName, err := names.ParseSpaceTag(entity.Tag)
		if err != nil {
			results[i].Error = common.ServerError(errors.Trace(err))
			continue
		}
		var result params.ShowSpaceResult
		space, err := api.backing.SpaceByName(spaceName.Id())
		if err != nil {
			newErr := errors.Annotatef(err, "fetching space %q", spaceName)
			results[i].Error = common.ServerError(newErr)
			continue
		}
		result.Space.Name = space.Name()
		result.Space.Id = space.Id()
		subnets, err := space.Subnets()
		if err != nil {
			newErr := errors.Annotatef(err, "fetching subnets")
			results[i].Error = common.ServerError(newErr)
			continue
		}

		result.Space.Subnets = make([]params.Subnet, len(subnets))
		for i, subnet := range subnets {
			result.Space.Subnets[i] = networkingcommon.BackingSubnetToParamsSubnet(subnet)
		}

		applications, err := api.getApplicationsBindSpace(space.Id())
		if err != nil {
			newErr := errors.Annotatef(err, "fetching applications")
			results[i].Error = common.ServerError(newErr)
			continue
		}
		result.Applications = applications

		machineCount, err := api.getMachineCountBySpaceID(space.Id())
		if err != nil {
			newErr := errors.Annotatef(err, "fetching machine count")
			results[i].Error = common.ServerError(newErr)
			continue
		}

		result.MachineCount = machineCount
		results[i] = result
	}

	return params.ShowSpaceResults{Results: results}, err
}

// ReloadSpaces is not available via the V2 API.
func (u *APIv2) ReloadSpaces(_, _ struct{}) {}

// ReloadSpaces refreshes spaces from substrate
func (api *API) ReloadSpaces() error {
	canWrite, err := api.auth.HasPermission(permission.WriteAccess, api.backing.ModelTag())
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

// checkSupportForProviderSpaces checks if the environment implements NetworkingEnviron
// and also if it support provider spaces. Returns an error if it does support provider spaces.
// We don't want to update/change provider sources spaces.
func (api *API) checkSupportForProviderSpaces() error {
	env, err := environs.GetEnviron(api.backing, environs.New)
	if err != nil {
		return errors.Annotate(err, "getting environ")
	}
	if environs.SupportsProviderSpaces(api.context, env) {
		return errors.NotSupportedf("renaming provider-sourced spaces")
	}
	return nil
}

func (api *API) getMachineCountBySpaceID(spaceID string) (int, error) {
	var count int
	machines, err := api.backing.AllMachines()
	if err != nil {
		return 0, errors.Trace(err)
	}
	for _, machine := range machines {
		spacesSet, err := machine.AllSpaces()
		if err != nil {
			return 0, errors.Trace(err)
		}
		if spacesSet.Contains(spaceID) {
			count++
		}
	}
	return count, nil
}

func (api *API) getApplicationsBindSpace(givenSpaceID string) ([]string, error) {
	endpointBindings, err := api.backing.AllEndpointBindings()
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Using a set as we only we want the application names once.
	applications := set.NewStrings()
	for _, binding := range endpointBindings {
		for _, spaceID := range binding.Bindings {
			if spaceID == givenSpaceID {
				applications.Add(binding.AppName)
			}
		}
	}
	return applications.SortedValues(), nil
}

func (api *API) checkSpacesCRUDPermissions() error {
	isAdmin, err := api.auth.HasPermission(permission.AdminAccess, api.backing.ModelTag())
	if err != nil && !errors.IsNotFound(err) {
		return errors.Trace(err)
	}
	if !isAdmin {
		return common.ServerError(common.ErrPerm)
	}
	if err := api.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	if err = api.checkSupportForProviderSpaces(); err != nil {
		return common.ServerError(errors.Trace(err))
	}
	return nil
}

func (api *API) getValidSubnetsByCIDR(CIDRs []string) ([]networkingcommon.BackingSubnet, error) {
	subnets := make([]networkingcommon.BackingSubnet, len(CIDRs))
	for i, cidr := range CIDRs {
		if !network.IsValidCidr(cidr) {
			return nil, errors.New(fmt.Sprintf("%q is not a valid CIDR", cidr))
		}
		subnet, err := api.backing.SubnetByCIDR(cidr)
		if err != nil {
			return nil, err
		}
		subnets[i] = subnet
	}
	return subnets, nil
}
