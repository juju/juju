// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner

import (
	"fmt"

	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	charmscommon "github.com/juju/juju/api/common/charms"
	apiwatcher "github.com/juju/juju/api/watcher"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/docker"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/storage"
)

// Client allows access to the CAAS application provisioner API endpoint.
type Client struct {
	facade base.FacadeCaller
	*charmscommon.CharmInfoClient
	*charmscommon.ApplicationCharmInfoClient
}

// NewClient returns a client used to access the CAAS Application Provisioner API.
func NewClient(caller base.APICaller) *Client {
	facadeCaller := base.NewFacadeCaller(caller, "CAASApplicationProvisioner")
	charmInfoClient := charmscommon.NewCharmInfoClient(facadeCaller)
	appCharmInfoClient := charmscommon.NewApplicationCharmInfoClient(facadeCaller)
	return &Client{
		facade:                     facadeCaller,
		CharmInfoClient:            charmInfoClient,
		ApplicationCharmInfoClient: appCharmInfoClient,
	}
}

// WatchApplications returns a StringsWatcher that notifies of
// changes to the lifecycles of CAAS applications in the current model.
func (c *Client) WatchApplications() (watcher.StringsWatcher, error) {
	var result params.StringsWatchResult
	if err := c.facade.FacadeCall("WatchApplications", nil, &result); err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewStringsWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

// SetPassword sets API password for the specified application.
func (c *Client) SetPassword(appName string, password string) error {
	var result params.ErrorResults
	args := params.EntityPasswords{Changes: []params.EntityPassword{{
		Tag:      names.NewApplicationTag(appName).String(),
		Password: password,
	}}}
	err := c.facade.FacadeCall("SetPasswords", args, &result)
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

// Life returns the lifecycle state for the specified application
// or unit in the current model.
func (c *Client) Life(entityName string) (life.Value, error) {
	var tag names.Tag
	switch {
	case names.IsValidApplication(entityName):
		tag = names.NewApplicationTag(entityName)
	case names.IsValidUnit(entityName):
		tag = names.NewUnitTag(entityName)
	default:
		return "", errors.NotValidf("application or unit name %q", entityName)
	}
	args := params.Entities{
		Entities: []params.Entity{{Tag: tag.String()}},
	}

	var results params.LifeResults
	if err := c.facade.FacadeCall("Life", args, &results); err != nil {
		return "", err
	}
	if n := len(results.Results); n != 1 {
		return "", errors.Errorf("expected 1 result, got %d", n)
	}
	if err := results.Results[0].Error; err != nil {
		return "", params.TranslateWellKnownError(err)
	}
	return results.Results[0].Life, nil
}

func (c *Client) WatchProvisioningInfo(applicationName string) (watcher.NotifyWatcher, error) {
	args := params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewApplicationTag(applicationName).String()},
		},
	}
	var results params.NotifyWatchResults

	if err := c.facade.FacadeCall("WatchProvisioningInfo", args, &results); err != nil {
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
	Version                   version.Number
	APIAddresses              []string
	CACert                    string
	Tags                      map[string]string
	Constraints               constraints.Value
	Filesystems               []storage.KubernetesFilesystemParams
	FilesystemUnitAttachments map[string][]storage.KubernetesFilesystemUnitAttachmentParams
	Devices                   []devices.KubernetesDeviceParams
	Base                      corebase.Base
	ImageDetails              resources.DockerImageDetails
	CharmModifiedVersion      int
	CharmURL                  *charm.URL
	Trust                     bool
	Scale                     int
}

// ProvisioningInfo returns the info needed to provision an operator for an application.
func (c *Client) ProvisioningInfo(applicationName string) (ProvisioningInfo, error) {
	args := params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewApplicationTag(applicationName).String()},
		},
	}
	var result params.CAASApplicationProvisioningInfoResults
	if err := c.facade.FacadeCall("ProvisioningInfo", args, &result); err != nil {
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
	for _, fs := range r.Filesystems {
		f, err := filesystemFromParams(fs)
		if err != nil {
			return info, errors.Trace(err)
		}
		info.Filesystems = append(info.Filesystems, *f)
	}

	fsUnitAttachments, err := filesystemUnitAttachmentsFromParams(r.FilesystemUnitAttachments)
	if err != nil {
		return info, errors.Trace(err)
	}
	info.FilesystemUnitAttachments = fsUnitAttachments

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

func filesystemFromParams(in params.KubernetesFilesystemParams) (*storage.KubernetesFilesystemParams, error) {
	var attachment *storage.KubernetesFilesystemAttachmentParams
	if in.Attachment != nil {
		var err error
		attachment, err = filesystemAttachmentFromParams(*in.Attachment)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	return &storage.KubernetesFilesystemParams{
		StorageName:  in.StorageName,
		Provider:     storage.ProviderType(in.Provider),
		Size:         in.Size,
		Attributes:   in.Attributes,
		ResourceTags: in.Tags,
		Attachment:   attachment,
	}, nil
}

func filesystemAttachmentFromParams(in params.KubernetesFilesystemAttachmentParams) (*storage.KubernetesFilesystemAttachmentParams, error) {
	return &storage.KubernetesFilesystemAttachmentParams{
		AttachmentParams: storage.AttachmentParams{
			Provider: storage.ProviderType(in.Provider),
			ReadOnly: in.ReadOnly,
		},
		Path: in.MountPoint,
	}, nil
}

func filesystemUnitAttachmentsFromParams(in map[string][]params.KubernetesFilesystemUnitAttachmentParams) (map[string][]storage.KubernetesFilesystemUnitAttachmentParams, error) {
	if len(in) == 0 {
		return nil, nil
	}

	k8sFsUnitAttachmentParams := make(map[string][]storage.KubernetesFilesystemUnitAttachmentParams)
	for storageName, params := range in {
		for _, p := range params {
			unitTag, err := names.ParseTag(p.UnitTag)
			if err != nil {
				return nil, errors.Trace(err)
			}
			k8sFsUnitAttachmentParams[storageName] = append(
				k8sFsUnitAttachmentParams[storageName],
				storage.KubernetesFilesystemUnitAttachmentParams{
					UnitName: unitTag.Id(),
					VolumeId: p.VolumeId,
				},
			)
		}
	}
	if len(k8sFsUnitAttachmentParams) == 0 {
		return nil, nil
	}
	return k8sFsUnitAttachmentParams, nil
}

// SetOperatorStatus updates the provisioning status of an operator.
func (c *Client) SetOperatorStatus(appName string, status status.Status, message string, data map[string]interface{}) error {
	var result params.ErrorResults
	args := params.SetStatus{Entities: []params.EntityStatusArgs{
		{Tag: names.NewApplicationTag(appName).String(), Status: status.String(), Info: message, Data: data},
	}}
	err := c.facade.FacadeCall("SetOperatorStatus", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// Units returns all the units for an Application.
func (c *Client) Units(appName string) ([]params.CAASUnit, error) {
	args := params.Entities{Entities: []params.Entity{{
		Tag: names.NewApplicationTag(appName).String(),
	}}}
	var result params.CAASUnitsResults
	if err := c.facade.FacadeCall("Units", args, &result); err != nil {
		return nil, errors.Trace(err)
	}
	if len(result.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d",
			len(result.Results))
	}
	res := result.Results[0]
	if res.Error != nil {
		return nil, errors.Annotatef(params.TranslateWellKnownError(res.Error), "unable to fetch units for %s", appName)
	}
	out := make([]params.CAASUnit, len(res.Units))
	for i, v := range res.Units {
		tag, err := names.ParseUnitTag(v.Tag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		out[i] = params.CAASUnit{
			Tag:        tag,
			UnitStatus: v.UnitStatus,
		}
	}
	return out, nil
}

// ApplicationOCIResources returns all the OCI image resources for an application.
func (c *Client) ApplicationOCIResources(appName string) (map[string]resources.DockerImageDetails, error) {
	args := params.Entities{Entities: []params.Entity{{
		Tag: names.NewApplicationTag(appName).String(),
	}}}
	var result params.CAASApplicationOCIResourceResults
	if err := c.facade.FacadeCall("ApplicationOCIResources", args, &result); err != nil {
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
	images := make(map[string]resources.DockerImageDetails)
	for k, v := range res.Result.Images {
		images[k] = resources.DockerImageDetails{
			RegistryPath: v.RegistryPath,
			ImageRepoDetails: docker.ImageRepoDetails{
				BasicAuthConfig: docker.BasicAuthConfig{
					Username: v.Username,
					Password: v.Password,
				},
			},
		}
	}
	return images, nil
}

// UpdateUnits updates the state model to reflect the state of the units
// as reported by the cloud.
func (c *Client) UpdateUnits(arg params.UpdateApplicationUnits) (*params.UpdateApplicationUnitsInfo, error) {
	var result params.UpdateApplicationUnitResults
	args := params.UpdateApplicationUnitArgs{Args: []params.UpdateApplicationUnits{arg}}
	err := c.facade.FacadeCall("UpdateApplicationsUnits", args, &result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(result.Results) != len(args.Args) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(args.Args), len(result.Results))
	}
	firstResult := result.Results[0]
	if firstResult.Error == nil {
		return firstResult.Info, nil
	}
	return firstResult.Info, params.TranslateWellKnownError(firstResult.Error)
}

// WatchApplication returns a NotifyWatcher that notifies of
// changes to the application in the current model.
func (c *Client) WatchApplication(appName string) (watcher.NotifyWatcher, error) {
	return common.Watch(c.facade, "Watch", names.NewApplicationTag(appName))
}

// ClearApplicationResources clears the flag which indicates an
// application still has resources in the cluster.
func (c *Client) ClearApplicationResources(appName string) error {
	var result params.ErrorResults
	args := params.Entities{Entities: []params.Entity{{Tag: names.NewApplicationTag(appName).String()}}}
	if err := c.facade.FacadeCall("ClearApplicationsResources", args, &result); err != nil {
		return errors.Trace(err)
	}
	if len(result.Results) != len(args.Entities) {
		return errors.Errorf("expected %d result(s), got %d", len(args.Entities), len(result.Results))
	}
	if result.Results[0].Error == nil {
		return nil
	}
	return params.TranslateWellKnownError(result.Results[0].Error)
}

// WatchUnits returns a StringsWatcher that notifies of
// changes to the lifecycles of units of the specified
// application in the current model.
func (c *Client) WatchUnits(application string) (watcher.StringsWatcher, error) {
	if !names.IsValidApplication(application) {
		return nil, errors.NotValidf("application name %q", application)
	}
	tag := names.NewApplicationTag(application)
	args := params.Entities{
		Entities: []params.Entity{{Tag: tag.String()}},
	}

	var results params.StringsWatchResults
	if err := c.facade.FacadeCall("WatchUnits", args, &results); err != nil {
		return nil, err
	}
	if n := len(results.Results); n != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", n)
	}
	if err := results.Results[0].Error; err != nil {
		return nil, errors.Trace(err)
	}
	w := apiwatcher.NewStringsWatcher(c.facade.RawAPICaller(), results.Results[0])
	return w, nil
}

// RemoveUnit removes the specified unit from the current model.
func (c *Client) RemoveUnit(unitName string) error {
	if !names.IsValidUnit(unitName) {
		return errors.NotValidf("unit name %q", unitName)
	}
	var result params.ErrorResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: names.NewUnitTag(unitName).String()}},
	}
	err := c.facade.FacadeCall("Remove", args, &result)
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
func (c *Client) DestroyUnits(unitNames []string) error {
	args := params.DestroyUnitsParams{}
	args.Units = make([]params.DestroyUnitParams, 0, len(unitNames))

	for _, unitName := range unitNames {
		tag := names.NewUnitTag(unitName)
		args.Units = append(args.Units, params.DestroyUnitParams{
			UnitTag: tag.String(),
		})
	}
	result := params.DestroyUnitResults{}

	err := c.facade.FacadeCall("DestroyUnits", args, &result)
	if err != nil {
		return errors.Trace(err)
	}

	if len(result.Results) != len(unitNames) {
		return fmt.Errorf("expected %d results got %d", len(unitNames), len(result.Results))
	}

	for _, res := range result.Results {
		if res.Error != nil {
			return errors.Trace(apiservererrors.RestoreError(res.Error))
		}
	}

	return nil
}

// ProvisioningState returns the current provisioning state for the CAAS application.
// The result can be nil.
func (c *Client) ProvisioningState(appName string) (*params.CAASApplicationProvisioningState, error) {
	var result params.CAASApplicationProvisioningStateResult
	args := params.Entity{Tag: names.NewApplicationTag(appName).String()}
	err := c.facade.FacadeCall("ProvisioningState", args, &result)
	if err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, result.Error
	}
	return result.ProvisioningState, nil
}

// SetProvisioningState sets the provisioning state for the CAAS application.
func (c *Client) SetProvisioningState(appName string, state params.CAASApplicationProvisioningState) error {
	var result params.ErrorResult
	args := params.CAASApplicationProvisioningStateArg{
		Application:       params.Entity{Tag: names.NewApplicationTag(appName).String()},
		ProvisioningState: state,
	}
	err := c.facade.FacadeCall("SetProvisioningState", args, &result)
	if err != nil {
		return err
	}
	if result.Error != nil {
		return result.Error
	}
	return nil
}

// ProvisionerConfig returns the provisoner's configuration.
func (c *Client) ProvisionerConfig() (params.CAASApplicationProvisionerConfig, error) {
	var result params.CAASApplicationProvisionerConfigResult
	err := c.facade.FacadeCall("ProvisionerConfig", nil, &result)
	if err != nil {
		return params.CAASApplicationProvisionerConfig{}, err
	}
	if result.Error != nil {
		return params.CAASApplicationProvisionerConfig{}, result.Error
	}
	if result.ProvisionerConfig == nil {
		return params.CAASApplicationProvisionerConfig{}, nil
	}
	return *result.ProvisionerConfig, nil
}

// ProvisioningFilesystemInfo holds the filesystem info needed to provision an operator.
type FilesystemProvisioningInfo struct {
	Filesystems               []storage.KubernetesFilesystemParams
	FilesystemUnitAttachments map[string][]storage.KubernetesFilesystemUnitAttachmentParams
}

// ProvisioningInfo returns the filesystem info needed to provision an operator for an application.
func (c *Client) FilesystemProvisioningInfo(applicationName string) (FilesystemProvisioningInfo, error) {
	args := params.Entity{Tag: names.NewApplicationTag(applicationName).String()}
	var result params.CAASApplicationFilesystemProvisioningInfoResult
	if err := c.facade.FacadeCall("FilesystemProvisioningInfo", args, &result); err != nil {
		return FilesystemProvisioningInfo{}, err
	}
	info := FilesystemProvisioningInfo{}

	for _, fs := range result.Result.Filesystems {
		f, err := filesystemFromParams(fs)
		if err != nil {
			return info, errors.Trace(err)
		}
		info.Filesystems = append(info.Filesystems, *f)
	}

	fsUnitAttachments, err := filesystemUnitAttachmentsFromParams(result.Result.FilesystemUnitAttachments)
	if err != nil {
		return info, errors.Trace(err)
	}
	info.FilesystemUnitAttachments = fsUnitAttachments
	return info, nil
}
