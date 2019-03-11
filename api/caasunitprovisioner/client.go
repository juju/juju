// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/storage"
)

// Client allows access to the CAAS unit provisioner API endpoint.
type Client struct {
	facade base.FacadeCaller
}

// NewClient returns a client used to access the CAAS unit provisioner API.
func NewClient(caller base.APICaller) *Client {
	facadeCaller := base.NewFacadeCaller(caller, "CAASUnitProvisioner")
	return &Client{
		facade: facadeCaller,
	}
}

func applicationTag(application string) (names.ApplicationTag, error) {
	if !names.IsValidApplication(application) {
		return names.ApplicationTag{}, errors.NotValidf("application name %q", application)
	}
	return names.NewApplicationTag(application), nil
}

func entities(tags ...names.Tag) params.Entities {
	entities := params.Entities{
		Entities: make([]params.Entity, len(tags)),
	}
	for i, tag := range tags {
		entities.Entities[i].Tag = tag.String()
	}
	return entities
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

// ApplicationConfig returns the config for the specified application.
func (c *Client) ApplicationConfig(applicationName string) (application.ConfigAttributes, error) {
	var results params.ApplicationGetConfigResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: names.NewApplicationTag(applicationName).String()}},
	}
	err := c.facade.FacadeCall("ApplicationsConfig", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != len(args.Entities) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(args.Entities), len(results.Results))
	}
	return application.ConfigAttributes(results.Results[0].Config), nil
}

// WatchApplicationScale returns a NotifyWatcher that notifies of
// changes to the lifecycles of units of the specified
// CAAS application in the current model.
func (c *Client) WatchApplicationScale(application string) (watcher.NotifyWatcher, error) {
	applicationTag, err := applicationTag(application)
	if err != nil {
		return nil, errors.Trace(err)
	}
	args := entities(applicationTag)

	var results params.NotifyWatchResults
	if err := c.facade.FacadeCall("WatchApplicationsScale", args, &results); err != nil {
		return nil, err
	}
	if n := len(results.Results); n != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", n)
	}
	if err := results.Results[0].Error; err != nil {
		return nil, errors.Trace(err)
	}
	w := apiwatcher.NewNotifyWatcher(c.facade.RawAPICaller(), results.Results[0])
	return w, nil
}

// ApplicationScale returns the scale for the specified application.
func (c *Client) ApplicationScale(applicationName string) (int, error) {
	var results params.IntResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: names.NewApplicationTag(applicationName).String()}},
	}
	err := c.facade.FacadeCall("ApplicationsScale", args, &results)
	if err != nil {
		return 0, errors.Trace(err)
	}
	if len(results.Results) != len(args.Entities) {
		return 0, errors.Errorf("expected %d result(s), got %d", len(args.Entities), len(results.Results))
	}
	return results.Results[0].Result, nil
}

// WatchPodSpec returns a NotifyWatcher that notifies of
// changes to the pod spec of the specified CAAS application in
// the current model.
func (c *Client) WatchPodSpec(application string) (watcher.NotifyWatcher, error) {
	appTag, err := applicationTag(application)
	if err != nil {
		return nil, errors.Trace(err)
	}
	args := entities(appTag)

	var results params.NotifyWatchResults
	if err := c.facade.FacadeCall("WatchPodSpec", args, &results); err != nil {
		return nil, err
	}
	if n := len(results.Results); n != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", n)
	}
	if err := results.Results[0].Error; err != nil {
		return nil, errors.Trace(err)
	}
	w := apiwatcher.NewNotifyWatcher(c.facade.RawAPICaller(), results.Results[0])
	return w, nil
}

// ProvisioningInfo holds unit provisioning info.
type ProvisioningInfo struct {
	PodSpec     string
	Constraints constraints.Value
	Filesystems []storage.KubernetesFilesystemParams
	Devices     []devices.KubernetesDeviceParams
	Tags        map[string]string
}

// ErrNoUnits is returned when trying to provision a caas app but
// there are no units defined in the model.
var ErrNoUnits = errors.New("no units to provision")

// ProvisioningInfo returns the provisioning info for the specified CAAS
// application in the current model.
func (c *Client) ProvisioningInfo(appName string) (*ProvisioningInfo, error) {
	appTag, err := applicationTag(appName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	args := entities(appTag)

	var results params.KubernetesProvisioningInfoResults
	if err := c.facade.FacadeCall("ProvisioningInfo", args, &results); err != nil {
		return nil, err
	}
	if n := len(results.Results); n != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", n)
	}
	if err := results.Results[0].Error; err != nil {
		return nil, maybeNotFound(err)
	}
	result := results.Results[0].Result
	if result == nil {
		return nil, ErrNoUnits
	}
	info := &ProvisioningInfo{
		PodSpec:     result.PodSpec,
		Constraints: result.Constraints,
		Tags:        result.Tags,
	}

	for _, fs := range result.Filesystems {
		fsInfo, err := filesystemFromParams(fs)
		if err != nil {
			return nil, errors.Trace(err)
		}
		info.Filesystems = append(info.Filesystems, *fsInfo)
	}

	var devs []devices.KubernetesDeviceParams
	for _, device := range result.Devices {
		devs = append(devs, devices.KubernetesDeviceParams{
			Type:       devices.DeviceType(device.Type),
			Count:      device.Count,
			Attributes: device.Attributes,
		})
	}
	info.Devices = devs
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

// Life returns the lifecycle state for the specified CAAS application
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
	args := entities(tag)

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
	return life.Value(results.Results[0].Life), nil
}

// maybeNotFound returns an error satisfying errors.IsNotFound
// if the supplied error has a CodeNotFound error.
func maybeNotFound(err *params.Error) error {
	if err == nil || !params.IsCodeNotFound(err) {
		return err
	}
	return errors.NewNotFound(err, "")
}

// UpdateUnits updates the state model to reflect the state of the units
// as reported by the cloud.
func (c *Client) UpdateUnits(arg params.UpdateApplicationUnits) error {
	var result params.ErrorResults
	args := params.UpdateApplicationUnitArgs{Args: []params.UpdateApplicationUnits{arg}}
	err := c.facade.FacadeCall("UpdateApplicationsUnits", args, &result)
	if err != nil {
		return errors.Trace(err)
	}
	if len(result.Results) != len(args.Args) {
		return errors.Errorf("expected %d result(s), got %d", len(args.Args), len(result.Results))
	}
	if result.Results[0].Error == nil {
		return nil
	}
	return maybeNotFound(result.Results[0].Error)
}

// UpdateApplicationService updates the state model to reflect the state of the application's
// service as reported by the cloud.
func (c *Client) UpdateApplicationService(arg params.UpdateApplicationServiceArg) error {
	var result params.ErrorResults
	args := params.UpdateApplicationServiceArgs{Args: []params.UpdateApplicationServiceArg{arg}}
	if err := c.facade.FacadeCall("UpdateApplicationsService", args, &result); err != nil {
		return errors.Trace(err)
	}
	if len(result.Results) != len(args.Args) {
		return errors.Errorf("expected %d result(s), got %d", len(args.Args), len(result.Results))
	}
	if result.Results[0].Error == nil {
		return nil
	}
	return maybeNotFound(result.Results[0].Error)
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
