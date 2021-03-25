// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner

import (
	"github.com/juju/charm/v9"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/version/v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	charmscommon "github.com/juju/juju/api/common/charms"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/storage"
)

// Client allows access to the CAAS application provisioner API endpoint.
type Client struct {
	facade base.FacadeCaller
	*charmscommon.CharmsClient
}

// NewClient returns a client used to access the CAAS Application Provisioner API.
func NewClient(caller base.APICaller) *Client {
	facadeCaller := base.NewFacadeCaller(caller, "CAASApplicationProvisioner")
	charmsClient := charmscommon.NewCharmsClient(facadeCaller)
	return &Client{
		facade:       facadeCaller,
		CharmsClient: charmsClient,
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
		return errors.Trace(maybeNotFound(result.Results[0].Error))
	}
	return nil
}

// maybeNotFound returns an error satisfying errors.IsNotFound
// if the supplied error has a CodeNotFound error.
func maybeNotFound(err *params.Error) error {
	if err == nil || !params.IsCodeNotFound(err) {
		return err
	}
	return errors.NewNotFound(err, "")
}

// Life returns the lifecycle state for the specified CAAS application
// or unit in the current model.
func (c *Client) Life(appName string) (life.Value, error) {
	if !names.IsValidApplication(appName) {
		return "", errors.NotValidf("application name %q", appName)
	}
	args := params.Entities{
		Entities: []params.Entity{{Tag: names.NewApplicationTag(appName).String()}},
	}

	var results params.LifeResults
	if err := c.facade.FacadeCall("Life", args, &results); err != nil {
		return "", err
	}
	if n := len(results.Results); n != 1 {
		return "", errors.Errorf("expected 1 result, got %d", n)
	}
	if err := results.Results[0].Error; err != nil {
		return "", maybeNotFound(err)
	}
	return results.Results[0].Life, nil
}

// ProvisioningInfo holds the info needed to provision an operator.
type ProvisioningInfo struct {
	ImagePath            string
	Version              version.Number
	APIAddresses         []string
	CACert               string
	Tags                 map[string]string
	Constraints          constraints.Value
	Filesystems          []storage.KubernetesFilesystemParams
	Devices              []devices.KubernetesDeviceParams
	Series               string
	ImageRepo            string
	CharmModifiedVersion int
	CharmURL             *charm.URL
}

// ProvisioningInfo returns the info needed to provision an operator for an application.
func (c *Client) ProvisioningInfo(applicationName string) (ProvisioningInfo, error) {
	args := params.Entities{[]params.Entity{
		{Tag: names.NewApplicationTag(applicationName).String()},
	}}
	var result params.CAASApplicationProvisioningInfoResults
	if err := c.facade.FacadeCall("ProvisioningInfo", args, &result); err != nil {
		return ProvisioningInfo{}, err
	}
	if len(result.Results) != 1 {
		return ProvisioningInfo{}, errors.Errorf("expected one result, got %d", len(result.Results))
	}
	r := result.Results[0]
	if err := r.Error; err != nil {
		return ProvisioningInfo{}, errors.Trace(maybeNotFound(err))
	}

	info := ProvisioningInfo{
		ImagePath:            r.ImagePath,
		Version:              r.Version,
		APIAddresses:         r.APIAddresses,
		CACert:               r.CACert,
		Tags:                 r.Tags,
		Constraints:          r.Constraints,
		Series:               r.Series,
		ImageRepo:            r.ImageRepo,
		CharmModifiedVersion: r.CharmModifiedVersion,
	}

	for _, fs := range r.Filesystems {
		f, err := filesystemFromParams(fs)
		if err != nil {
			return info, errors.Trace(err)
		}
		info.Filesystems = append(info.Filesystems, *f)
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

// ApplicationCharmURL finds the CharmURL for an application.
func (c *Client) ApplicationCharmURL(appName string) (*charm.URL, error) {
	args := params.Entities{Entities: []params.Entity{{
		Tag: names.NewApplicationTag(appName).String(),
	}}}
	var result params.StringResults
	if err := c.facade.FacadeCall("ApplicationCharmURLs", args, &result); err != nil {
		return nil, errors.Trace(err)
	}
	if len(result.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d",
			len(result.Results))
	}
	res := result.Results[0]
	if res.Error != nil {
		return nil, errors.Annotatef(maybeNotFound(res.Error), "unable to fetch charm url for %s", appName)
	}
	url, err := charm.ParseURL(res.Result)
	if err != nil {
		return nil, errors.Annotatef(err, "invalid charm url %q", res.Result)
	}
	return url, nil
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
func (c *Client) Units(appName string) ([]names.Tag, error) {
	args := params.Entities{Entities: []params.Entity{{
		Tag: names.NewApplicationTag(appName).String(),
	}}}
	var result params.EntitiesResults
	if err := c.facade.FacadeCall("Units", args, &result); err != nil {
		return nil, errors.Trace(err)
	}
	if len(result.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d",
			len(result.Results))
	}
	res := result.Results[0]
	if res.Error != nil {
		return nil, errors.Annotatef(maybeNotFound(res.Error), "unable to fetch units for %s", appName)
	}
	tags := make([]names.Tag, 0, len(res.Entities))
	for _, v := range res.Entities {
		tag, err := names.ParseUnitTag(v.Tag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		tags = append(tags, tag)
	}
	return tags, nil
}

// GarbageCollect cleans up units that have gone away permanently.
// Only observed units will be deleted as new units could have surfaced between
// the capturing of kubernetes pod state/application state and this call.
func (c *Client) GarbageCollect(
	appName string, observedUnits []names.Tag, desiredReplicas int, activePodNames []string, force bool) error {
	var result params.ErrorResults
	observedEntities := params.Entities{
		Entities: make([]params.Entity, len(observedUnits)),
	}
	for i, v := range observedUnits {
		observedEntities.Entities[i].Tag = v.String()
	}
	args := params.CAASApplicationGarbageCollectArgs{Args: []params.CAASApplicationGarbageCollectArg{{
		Application:     params.Entity{Tag: names.NewApplicationTag(appName).String()},
		ObservedUnits:   observedEntities,
		DesiredReplicas: desiredReplicas,
		ActivePodNames:  activePodNames,
		Force:           force,
	}}}
	err := c.facade.FacadeCall("CAASApplicationGarbageCollect", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
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
		return nil, errors.Annotatef(maybeNotFound(res.Error), "unable to fetch OCI image resources for %s", appName)
	}
	if res.Result == nil {
		return nil, errors.Errorf("missing result")
	}
	images := make(map[string]resources.DockerImageDetails)
	for k, v := range res.Result.Images {
		images[k] = resources.DockerImageDetails{
			RegistryPath: v.RegistryPath,
			Username:     v.Username,
			Password:     v.Password,
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
	return firstResult.Info, maybeNotFound(firstResult.Error)
}

// WatchApplication returns a NotifyWatcher that notifies of
// changes to the application in the current model.
func (c *Client) WatchApplication(appName string) (watcher.NotifyWatcher, error) {
	return common.Watch(c.facade, "Watch", names.NewApplicationTag(appName))
}
