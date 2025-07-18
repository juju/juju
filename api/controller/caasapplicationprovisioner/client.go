// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api/base"
	charmscommon "github.com/juju/juju/api/common/charms"
	apiwatcher "github.com/juju/juju/api/watcher"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

// Client allows access to the CAAS application provisioner API endpoint.
type Client struct {
	facade base.FacadeCaller
	*charmscommon.CharmInfoClient
	*charmscommon.ApplicationCharmInfoClient
}

// NewClient returns a client used to access the CAAS Application Provisioner API.
func NewClient(caller base.APICaller, options ...Option) *Client {
	facadeCaller := base.NewFacadeCaller(caller, "CAASApplicationProvisioner", options...)
	charmInfoClient := charmscommon.NewCharmInfoClient(facadeCaller)
	appCharmInfoClient := charmscommon.NewApplicationCharmInfoClient(facadeCaller)
	return &Client{
		facade:                     facadeCaller,
		CharmInfoClient:            charmInfoClient,
		ApplicationCharmInfoClient: appCharmInfoClient,
	}
}

// SetPassword sets API password for the specified application.
func (c *Client) SetPassword(ctx context.Context, appName string, password string) error {
	var result params.ErrorResults
	args := params.EntityPasswords{Changes: []params.EntityPassword{{
		Tag:      names.NewApplicationTag(appName).String(),
		Password: password,
	}}}
	err := c.facade.FacadeCall(ctx, "SetPasswords", args, &result)
	if err != nil {
		return err
	}
	if len(result.Results) != 1 {
		return errors.Errorf("invalid number of results %d expected 1", len(result.Results))
	}
	if result.Results[0].Error != nil {
		return errors.Trace(params.TranslateWellKnownError(result.Results[0].Error))
	}
	return nil
}

func (c *Client) WatchProvisioningInfo(ctx context.Context, applicationName string) (watcher.NotifyWatcher, error) {
	args := params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewApplicationTag(applicationName).String()},
		},
	}
	var results params.NotifyWatchResults

	if err := c.facade.FacadeCall(ctx, "WatchProvisioningInfo", args, &results); err != nil {
		return nil, err
	}

	if len(results.Results) != 1 {
		return nil, fmt.Errorf("expected 1 result when watching provisioning info for application %q", applicationName)
	}

	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}

	return apiwatcher.NewNotifyWatcher(c.facade.RawAPICaller(), result), nil
}

// ProvisioningInfo holds the info needed to provision an operator.
type ProvisioningInfo struct {
	Version              semversion.Number
	APIAddresses         []string
	CACert               string
	Tags                 map[string]string
	Constraints          constraints.Value
	Devices              []devices.KubernetesDeviceParams
	Base                 corebase.Base
	ImageDetails         resource.DockerImageDetails
	CharmModifiedVersion int
	CharmURL             *charm.URL
	Trust                bool
	Scale                int
}

// ProvisioningInfo returns the info needed to provision an operator for an application.
func (c *Client) ProvisioningInfo(ctx context.Context, applicationName string) (ProvisioningInfo, error) {
	args := params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewApplicationTag(applicationName).String()},
		},
	}
	var result params.CAASApplicationProvisioningInfoResults
	if err := c.facade.FacadeCall(ctx, "ProvisioningInfo", args, &result); err != nil {
		return ProvisioningInfo{}, err
	}
	if len(result.Results) != 1 {
		return ProvisioningInfo{}, errors.Errorf("expected one result, got %d", len(result.Results))
	}
	r := result.Results[0]
	if err := r.Error; err != nil {
		return ProvisioningInfo{}, errors.Trace(params.TranslateWellKnownError(err))
	}

	base, err := corebase.ParseBase(r.Base.Name, r.Base.Channel)
	if err != nil {
		return ProvisioningInfo{}, errors.Trace(err)
	}
	info := ProvisioningInfo{
		Version:              r.Version,
		APIAddresses:         r.APIAddresses,
		CACert:               r.CACert,
		Tags:                 r.Tags,
		Constraints:          r.Constraints,
		Base:                 base,
		ImageDetails:         params.ConvertDockerImageInfo(r.ImageRepo),
		CharmModifiedVersion: r.CharmModifiedVersion,
		Trust:                r.Trust,
		Scale:                r.Scale,
	}

	for _, device := range r.Devices {
		info.Devices = append(info.Devices, devices.KubernetesDeviceParams{
			Type:       devices.DeviceType(device.Type),
			Count:      device.Count,
			Attributes: device.Attributes,
		})
	}

	if r.CharmURL != "" {
		charmURL, err := charm.ParseURL(r.CharmURL)
		if err != nil {
			return info, errors.Trace(err)
		}
		info.CharmURL = charmURL
	}

	return info, nil
}

// ApplicationOCIResources returns all the OCI image resources for an application.
func (c *Client) ApplicationOCIResources(ctx context.Context, appName string) (map[string]resource.DockerImageDetails, error) {
	args := params.Entities{Entities: []params.Entity{{
		Tag: names.NewApplicationTag(appName).String(),
	}}}
	var result params.CAASApplicationOCIResourceResults
	if err := c.facade.FacadeCall(ctx, "ApplicationOCIResources", args, &result); err != nil {
		return nil, errors.Trace(err)
	}
	if len(result.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d",
			len(result.Results))
	}
	res := result.Results[0]
	if res.Error != nil {
		return nil, errors.Annotatef(params.TranslateWellKnownError(res.Error), "unable to fetch OCI image resources for %s", appName)
	}
	if res.Result == nil {
		return nil, errors.Errorf("missing result")
	}
	images := make(map[string]resource.DockerImageDetails)
	for k, v := range res.Result.Images {
		images[k] = resource.DockerImageDetails{
			RegistryPath: v.RegistryPath,
			ImageRepoDetails: resource.ImageRepoDetails{
				BasicAuthConfig: resource.BasicAuthConfig{
					Username: v.Username,
					Password: v.Password,
				},
			},
		}
	}
	return images, nil
}

// RemoveUnit removes the specified unit from the current model.
func (c *Client) RemoveUnit(ctx context.Context, unitName string) error {
	if !names.IsValidUnit(unitName) {
		return errors.NotValidf("unit name %q", unitName)
	}
	var result params.ErrorResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: names.NewUnitTag(unitName).String()}},
	}
	err := c.facade.FacadeCall(ctx, "Remove", args, &result)
	if err != nil {
		return err
	}
	resultErr := result.OneError()
	if params.IsCodeNotFound(resultErr) {
		return nil
	}
	return resultErr
}

// DestroyUnits is responsible for starting the process of destroying units
// associated with this application.
func (c *Client) DestroyUnits(ctx context.Context, unitNames []string) error {
	args := params.DestroyUnitsParams{}
	args.Units = make([]params.DestroyUnitParams, 0, len(unitNames))

	for _, unitName := range unitNames {
		tag := names.NewUnitTag(unitName)
		args.Units = append(args.Units, params.DestroyUnitParams{
			UnitTag: tag.String(),
		})
	}
	result := params.DestroyUnitResults{}

	err := c.facade.FacadeCall(ctx, "DestroyUnits", args, &result)
	if err != nil {
		return errors.Trace(err)
	}

	if len(result.Results) != len(unitNames) {
		return fmt.Errorf("expected %d results got %d", len(unitNames), len(result.Results))
	}

	for _, res := range result.Results {
		if res.Error != nil {
			return errors.Trace(params.TranslateWellKnownError(res.Error))
		}
	}

	return nil
}
