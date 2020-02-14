// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/network"
)

const spacesFacade = "Spaces"

// API provides access to the InstancePoller API facade.
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
func (api *API) ShowSpace(name string) (network.ShowSpace, error) {
	var response params.ShowSpaceResults
	var result network.ShowSpace
	var args interface{}
	args = params.Entities{
		Entities: []params.Entity{{Tag: names.NewSpaceTag(name).String()}},
	}
	err := api.facade.FacadeCall("ShowSpace", args, &response)
	if err != nil {
		if params.IsCodeNotSupported(err) {
			return result, errors.NewNotSupported(nil, err.Error())
		}
		return result, errors.Trace(err)
	}
	if len(response.Results) != 1 {
		return result, errors.Errorf("expected 1 result, got %d", len(response.Results))
	}
	if err := response.Results[0].Error; err != nil {
		return result, err
	}
	convertedSpaceResult := ShowSpaceFromResult(response.Results[0])
	return convertedSpaceResult, err
}

// ShowSpaceFromResult converts params.ShowSpaceResult to network.ShowSpace
func ShowSpaceFromResult(result params.ShowSpaceResult) network.ShowSpace {
	s := result.Space
	subnets := make([]network.SubnetInfo, len(s.Subnets))
	for i, value := range s.Subnets {
		subnets[i].AvailabilityZones = value.Zones
		subnets[i].ProviderId = network.Id(value.ProviderId)
		subnets[i].VLANTag = value.VLANTag
		subnets[i].CIDR = value.CIDR
		subnets[i].ProviderNetworkId = network.Id(value.ProviderNetworkId)
		subnets[i].ProviderSpaceId = network.Id(value.ProviderSpaceId)
	}
	space := network.ShowSpace{
		Space: network.SpaceInfo{
			ID:      s.Id,
			Name:    network.SpaceName(s.Name),
			Subnets: subnets,
		},
		Applications: result.Applications,
		MachineCount: result.MachineCount,
	}
	return space
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

func (api *API) RenameSpace(oldName string, newName string) error {
	var response params.ErrorResults
	spaceRenameParams := make([]params.RenameSpaceParams, 1)
	spaceRename := params.RenameSpaceParams{
		FromSpaceTag: names.NewSpaceTag(oldName).String(),
		ToSpaceTag:   names.NewSpaceTag(newName).String(),
	}
	spaceRenameParams[0] = spaceRename
	args := params.RenameSpacesParams{SpacesRenames: spaceRenameParams}
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
func (api *API) RemoveSpace(name string, model string, force bool, dryRun bool) (network.RemoveSpace, error) {
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
			return network.RemoveSpace{}, errors.NewNotSupported(nil, err.Error())
		}
		return network.RemoveSpace{}, errors.Trace(err)
	}
	if len(response.Results) == 0 {
		return network.RemoveSpace{}, nil
	}
	if len(response.Results) > 1 {
		return network.RemoveSpace{}, errors.Errorf("%d results, expected 0 or 1", len(response.Results))
	}

	for _, result := range response.Results {
		if result.Error != nil {
			return network.RemoveSpace{}, result.Error
		}
	}

	result := response.Results[0]

	constraints, err := convertEntitiesToString(result.Constraints, model)
	if err != nil {
		return network.RemoveSpace{}, err
	}
	bindings, err := convertEntitiesToString(result.Bindings, model)
	if err != nil {
		return network.RemoveSpace{}, err
	}

	return network.RemoveSpace{
		Space:            name,
		Constraints:      constraints,
		Bindings:         bindings,
		ControllerConfig: result.ControllerSettings,
	}, nil

}

func convertEntitiesToString(entities []params.Entity, currentModel string) ([]string, error) {
	var outputString []string
	for _, ent := range entities {
		tag, err := names.ParseTag(ent.Tag)
		if err != nil {
			return nil, err
		}
		if tag.Kind() == names.ModelTagKind {
			outputString = append(outputString, currentModel)
		} else {
			outputString = append(outputString, tag.Id())
		}
	}
	return outputString, nil
}
