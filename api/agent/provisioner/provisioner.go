// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"context"

	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/life"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/network"
	"github.com/juju/juju/internal/tools"
	"github.com/juju/juju/rpc/params"
)

// Client provides access to the Provisioner API facade.
type Client struct {
	*common.ModelWatcher
	*common.APIAddresser
	*common.ControllerConfigAPI

	facade base.FacadeCaller
}

// MachineResult provides a found Machine and any Error related to
// finding it.
type MachineResult struct {
	Machine MachineProvisioner
	Err     *params.Error
}

// MachineStatusResult provides a found Machine and Status Results
// for it.
type MachineStatusResult struct {
	Machine MachineProvisioner
	Status  params.StatusResult
}

// DistributionGroupResult provides a slice of machine.Ids in the
// distribution group and any Error related to finding it.
type DistributionGroupResult struct {
	MachineIds []string
	Err        *params.Error
}

// LXDProfileResult provides a charm.LXDProfile, adding the name.
type LXDProfileResult struct {
	Config      map[string]string            `json:"config" yaml:"config"`
	Description string                       `json:"description" yaml:"description"`
	Devices     map[string]map[string]string `json:"devices" yaml:"devices"`
	Name        string                       `json:"name" yaml:"name"`
}

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

const provisionerFacade = "Provisioner"

// NewClient creates a new provisioner facade using the input caller.
func NewClient(caller base.APICaller, options ...Option) *Client {
	facadeCaller := base.NewFacadeCaller(caller, provisionerFacade, options...)
	return &Client{
		ModelWatcher:        common.NewModelWatcher(facadeCaller),
		APIAddresser:        common.NewAPIAddresser(facadeCaller),
		ControllerConfigAPI: common.NewControllerConfig(facadeCaller),
		facade:              facadeCaller,
	}
}

// machineLife requests the lifecycle of the given machine from the server.
func (st *Client) machineLife(ctx context.Context, tag names.MachineTag) (life.Value, error) {
	return common.OneLife(ctx, st.facade, tag)
}

// ProvisioningInfo implements MachineProvisioner.ProvisioningInfo.
func (st *Client) ProvisioningInfo(machineTags []names.MachineTag) (params.ProvisioningInfoResults, error) {
	var results params.ProvisioningInfoResults
	args := params.Entities{Entities: transform.Slice(machineTags, func(t names.MachineTag) params.Entity {
		return params.Entity{Tag: t.String()}
	})}
	err := st.facade.FacadeCall(context.TODO(), "ProvisioningInfo", args, &results)
	return results, err
}

// Machines provides access to methods of a state.Machine through the facade
// for the given tags.
func (st *Client) Machines(ctx context.Context, tags ...names.MachineTag) ([]MachineResult, error) {
	lenTags := len(tags)
	genericTags := make([]names.Tag, lenTags)
	for i, t := range tags {
		genericTags[i] = t
	}
	result, err := common.Life(ctx, st.facade, genericTags)
	if err != nil {
		return []MachineResult{}, err
	}
	machines := make([]MachineResult, lenTags)
	for i, r := range result {
		if r.Error == nil {
			machines[i].Machine = &Machine{
				tag:  tags[i],
				life: r.Life,
				st:   st,
			}
		} else {
			machines[i].Err = r.Error
		}
	}
	return machines, nil
}

// WatchModelMachines returns a StringsWatcher that notifies of
// changes to the lifecycles of the machines (but not containers) in
// the current model.
func (st *Client) WatchModelMachines() (watcher.StringsWatcher, error) {
	var result params.StringsWatchResult
	err := st.facade.FacadeCall(context.TODO(), "WatchModelMachines", nil, &result)
	if err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewStringsWatcher(st.facade.RawAPICaller(), result)
	return w, nil
}

func (st *Client) WatchMachineErrorRetry() (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult
	err := st.facade.FacadeCall(context.TODO(), "WatchMachineErrorRetry", nil, &result)
	if err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewNotifyWatcher(st.facade.RawAPICaller(), result)
	return w, nil
}

// ContainerManagerConfig returns information from the model config that is
// needed for configuring the container manager.
func (st *Client) ContainerManagerConfig(args params.ContainerManagerConfigParams) (result params.ContainerManagerConfig, err error) {
	err = st.facade.FacadeCall(context.TODO(), "ContainerManagerConfig", args, &result)
	return result, err
}

// ContainerConfig returns information from the model config that is
// needed for container cloud-init.
func (st *Client) ContainerConfig() (result params.ContainerConfig, err error) {
	err = st.facade.FacadeCall(context.TODO(), "ContainerConfig", nil, &result)
	return result, err
}

// MachinesWithTransientErrors returns a slice of machines and corresponding status information
// for those machines which have transient provisioning errors.
func (st *Client) MachinesWithTransientErrors() ([]MachineStatusResult, error) {
	var results params.StatusResults
	err := st.facade.FacadeCall(context.TODO(), "MachinesWithTransientErrors", nil, &results)
	if err != nil {
		return []MachineStatusResult{}, err
	}
	machines := make([]MachineStatusResult, len(results.Results))
	for i, status := range results.Results {
		if status.Error != nil {
			continue
		}
		machines[i].Machine = &Machine{
			tag:  names.NewMachineTag(status.Id),
			life: status.Life,
			st:   st,
		}
		machines[i].Status = status
	}
	return machines, nil
}

// FindTools returns al ist of tools matching the specified version number and
// series, and, arch. If arch is blank, a default will be used.
func (st *Client) FindTools(v version.Number, os string, arch string) (tools.List, error) {
	args := params.FindToolsParams{
		Number: v,
		OSType: os,
	}
	if arch != "" {
		args.Arch = arch
	}
	var result params.FindToolsResult
	if err := st.facade.FacadeCall(context.TODO(), "FindTools", args, &result); err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, result.Error
	}
	return result.List, nil
}

// ReleaseContainerAddresses releases a static IP address allocated to a
// container.
func (st *Client) ReleaseContainerAddresses(containerTag names.MachineTag) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot release static addresses for %q", containerTag.Id())
	var result params.ErrorResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: containerTag.String()}},
	}
	if err := st.facade.FacadeCall(context.TODO(), "ReleaseContainerAddresses", args, &result); err != nil {
		return err
	}
	return result.OneError()
}

// PrepareContainerInterfaceInfo allocates an address and returns information to
// configure networking for a container. It accepts container tags as arguments.
func (st *Client) PrepareContainerInterfaceInfo(containerTag names.MachineTag) (corenetwork.InterfaceInfos, error) {
	var result params.MachineNetworkConfigResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: containerTag.String()}},
	}

	if err := st.facade.FacadeCall(context.TODO(), "PrepareContainerInterfaceInfo", args, &result); err != nil {
		return nil, err
	}
	if len(result.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(result.Results))
	}
	if err := result.Results[0].Error; err != nil {
		return nil, err
	}

	return params.InterfaceInfoFromNetworkConfig(result.Results[0].Config), nil
}

// SetHostMachineNetworkConfig sets the network configuration of the
// machine with netConfig
func (st *Client) SetHostMachineNetworkConfig(hostMachineTag names.MachineTag, netConfig []params.NetworkConfig) error {
	args := params.SetMachineNetworkConfig{
		Tag:    hostMachineTag.String(),
		Config: netConfig,
	}
	err := st.facade.FacadeCall(context.TODO(), "SetHostMachineNetworkConfig", args, nil)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (st *Client) HostChangesForContainer(containerTag names.MachineTag) ([]network.DeviceToBridge, int, error) {
	var result params.HostNetworkChangeResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: containerTag.String()}},
	}
	if err := st.facade.FacadeCall(context.TODO(), "HostChangesForContainers", args, &result); err != nil {
		return nil, 0, err
	}
	if len(result.Results) != 1 {
		return nil, 0, errors.Errorf("expected 1 result, got %d", len(result.Results))
	}
	if err := result.Results[0].Error; err != nil {
		return nil, 0, err
	}
	newBridges := result.Results[0].NewBridges
	res := make([]network.DeviceToBridge, len(newBridges))
	for i, bridgeInfo := range newBridges {
		res[i].BridgeName = bridgeInfo.BridgeName
		res[i].DeviceName = bridgeInfo.HostDeviceName
		res[i].MACAddress = bridgeInfo.MACAddress
	}
	return res, result.Results[0].ReconfigureDelay, nil
}

// DistributionGroupByMachineId returns a slice of machine.Ids
// that belong to the same distribution group as the given
// Machine. The provisioner may use this information
// to distribute instances for high availability.
func (st *Client) DistributionGroupByMachineId(tags ...names.MachineTag) ([]DistributionGroupResult, error) {
	var stringResults params.StringsResults
	entities := make([]params.Entity, len(tags))
	for i, t := range tags {
		entities[i] = params.Entity{Tag: t.String()}
	}
	err := st.facade.FacadeCall(context.TODO(), "DistributionGroupByMachineId", params.Entities{Entities: entities}, &stringResults)
	if err != nil {
		return []DistributionGroupResult{}, err
	}
	results := make([]DistributionGroupResult, len(tags))
	for i, stringResult := range stringResults.Results {
		results[i] = DistributionGroupResult{MachineIds: stringResult.Result, Err: stringResult.Error}
	}
	return results, nil
}

// CACert returns the certificate used to validate the API and state connections.
func (st *Client) CACert() (string, error) {
	var result params.BytesResult
	err := st.facade.FacadeCall(context.TODO(), "CACert", nil, &result)
	if err != nil {
		return "", err
	}
	return string(result.Result), nil
}

// GetContainerProfileInfo returns a slice of ContainerLXDProfile, 1 for each unit's charm
// which contains an lxd-profile.yaml.
func (st *Client) GetContainerProfileInfo(containerTag names.MachineTag) ([]*LXDProfileResult, error) {
	var result params.ContainerProfileResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: containerTag.String()}},
	}
	if err := st.facade.FacadeCall(context.TODO(), "GetContainerProfileInfo", args, &result); err != nil {
		return nil, err
	}
	if len(result.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(result.Results))
	}
	if err := result.Results[0].Error; err != nil {
		return nil, err
	}
	profiles := result.Results[0].LXDProfiles
	var res []*LXDProfileResult
	for _, p := range profiles {
		if p == nil {
			continue
		}
		res = append(res, &LXDProfileResult{
			Config:      p.Profile.Config,
			Description: p.Profile.Description,
			Devices:     p.Profile.Devices,
			Name:        p.Name,
		})
	}
	return res, nil
}

// ModelUUID returns the model UUID to connect to the model
// that the current connection is for.
func (st *Client) ModelUUID() (string, error) {
	var result params.StringResult
	err := st.facade.FacadeCall(context.TODO(), "ModelUUID", nil, &result)
	if err != nil {
		return "", err
	}
	return result.Result, nil
}
