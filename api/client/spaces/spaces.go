// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

const spacesFacade = "Spaces"

// API provides access to the Spaces API facade.
type API struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewAPI creates a new client-side Spaces facade.
func NewAPI(caller base.APICallCloser) *API {
	if caller == nil {
		panic("caller is nil")
	}
	clientFacade, facadeCaller := base.NewClientFacade(caller, spacesFacade)
	return &API{
		ClientFacade: clientFacade,
		facade:       facadeCaller,
	}
}

func makeCreateSpacesParamsV4(name string, cidrs []string, public bool) params.CreateSpacesParamsV4 {
	spaceTag := names.NewSpaceTag(name).String()
	subnetTags := make([]string, len(cidrs))

	for i, cidr := range cidrs {
		// For backwards compatibility, mimic old SubnetTags from names.v2.
		// The CIDR will be validated by the facade.
		subnetTags[i] = "subnet-" + cidr
	}

	return params.CreateSpacesParamsV4{
		Spaces: []params.CreateSpaceParamsV4{{
			SpaceTag:   spaceTag,
			SubnetTags: subnetTags,
			Public:     public,
		}}}
}

func makeCreateSpacesParams(name string, cidrs []string, public bool) params.CreateSpacesParams {
	return params.CreateSpacesParams{
		Spaces: []params.CreateSpaceParams{
			{
				SpaceTag: names.NewSpaceTag(name).String(),
				CIDRs:    cidrs,
				Public:   public,
			},
		},
	}
}

// CreateSpace creates a new Juju network space, associating the
// specified subnets with it (optional; can be empty).
func (api *API) CreateSpace(name string, cidrs []string, public bool) error {
	var response params.ErrorResults
	var args interface{}
	if bestVer := api.BestAPIVersion(); bestVer < 5 {
		args = makeCreateSpacesParamsV4(name, cidrs, public)
	} else {
		args = makeCreateSpacesParams(name, cidrs, public)
	}
	err := api.facade.FacadeCall("CreateSpaces", args, &response)
	if err != nil {
		if params.IsCodeNotSupported(err) {
			return errors.NewNotSupported(nil, err.Error())
		}
		return errors.Trace(err)
	}
	return response.OneError()
}

// ShowSpace shows details about a space.
// Containing subnets, applications and machines count associated with it.
func (api *API) ShowSpace(name string) (params.ShowSpaceResult, error) {
	var response params.ShowSpaceResults
	var args interface{}
	args = params.Entities{
		Entities: []params.Entity{{Tag: names.NewSpaceTag(name).String()}},
	}
	err := api.facade.FacadeCall("ShowSpace", args, &response)
	if err != nil {
		if params.IsCodeNotSupported(err) {
			return params.ShowSpaceResult{}, errors.NewNotSupported(nil, err.Error())
		}
		return params.ShowSpaceResult{}, errors.Trace(err)
	}
	if len(response.Results) != 1 {
		return params.ShowSpaceResult{}, errors.Errorf("expected 1 result, got %d", len(response.Results))
	}

	result := response.Results[0]
	if err := result.Error; err != nil {
		return params.ShowSpaceResult{}, errors.Trace(err)
	}
	return result, err
}

// ListSpaces lists all available spaces and their associated subnets.
func (api *API) ListSpaces() ([]params.Space, error) {
	var response params.ListSpacesResults
	err := api.facade.FacadeCall("ListSpaces", nil, &response)
	if params.IsCodeNotSupported(err) {
		return response.Results, errors.NewNotSupported(nil, err.Error())
	}
	return response.Results, err
}

// ReloadSpaces reloads spaces from substrate.
func (api *API) ReloadSpaces() error {
	if api.facade.BestAPIVersion() < 3 {
		return errors.NewNotSupported(nil, "Controller does not support reloading spaces")
	}
	err := api.facade.FacadeCall("ReloadSpaces", nil, nil)
	if params.IsCodeNotSupported(err) {
		return errors.NewNotSupported(nil, err.Error())
	}
	return err
}

// RenameSpace attempts to rename a space from the old name to a new name.
func (api *API) RenameSpace(oldName string, newName string) error {
	var response params.ErrorResults
	spaceRenameParams := make([]params.RenameSpaceParams, 1)
	spaceRename := params.RenameSpaceParams{
		FromSpaceTag: names.NewSpaceTag(oldName).String(),
		ToSpaceTag:   names.NewSpaceTag(newName).String(),
	}
	spaceRenameParams[0] = spaceRename
	args := params.RenameSpacesParams{Changes: spaceRenameParams}
	err := api.facade.FacadeCall("RenameSpace", args, &response)
	if err != nil {
		if params.IsCodeNotSupported(err) {
			return errors.NewNotSupported(nil, err.Error())
		}
		return errors.Trace(err)
	}
	if err := response.Combine(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// RemoveSpace removes a space.
func (api *API) RemoveSpace(name string, force bool, dryRun bool) (params.RemoveSpaceResult, error) {
	var response params.RemoveSpaceResults
	args := params.RemoveSpaceParams{
		SpaceParams: []params.RemoveSpaceParam{{
			Space:  params.Entity{Tag: names.NewSpaceTag(name).String()},
			Force:  force,
			DryRun: dryRun,
		}},
	}
	err := api.facade.FacadeCall("RemoveSpace", args, &response)
	if err != nil {
		if params.IsCodeNotSupported(err) {
			return params.RemoveSpaceResult{}, errors.NewNotSupported(nil, err.Error())
		}
		return params.RemoveSpaceResult{}, errors.Trace(err)
	}
	if len(response.Results) != 1 {
		return params.RemoveSpaceResult{}, errors.Errorf("%d results, expected 1", len(response.Results))
	}

	result := response.Results[0]
	if result.Error != nil {
		return params.RemoveSpaceResult{}, result.Error
	}
	return result, nil
}

// MoveSubnets ensures that the input subnets are in the input space.
func (api *API) MoveSubnets(space names.SpaceTag, subnets []names.SubnetTag, force bool) (params.MoveSubnetsResult, error) {
	subnetTags := make([]string, len(subnets))
	for k, subnet := range subnets {
		subnetTags[k] = subnet.String()
	}

	args := params.MoveSubnetsParams{
		Args: []params.MoveSubnetsParam{{
			SubnetTags: subnetTags,
			SpaceTag:   space.String(),
			Force:      force,
		}},
	}

	var results params.MoveSubnetsResults
	if err := api.facade.FacadeCall("MoveSubnets", args, &results); err != nil {
		if params.IsCodeNotSupported(err) {
			return params.MoveSubnetsResult{}, errors.NewNotSupported(nil, err.Error())
		}
		return params.MoveSubnetsResult{}, errors.Trace(err)
	}

	if len(results.Results) != 1 {
		return params.MoveSubnetsResult{}, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}

	result := results.Results[0]
	if result.Error != nil {
		return result, errors.Trace(result.Error)
	}

	return result, nil
}
