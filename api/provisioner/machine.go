// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"

	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/instance"
)

//go:generate mockgen -package mocks -destination mocks/machine_mock.go github.com/juju/juju/api/provisioner MachineProvisioner

// MachineProvisioner defines what provisioner needs to provision a machine.
type MachineProvisioner interface {
	// Tag returns the machine's tag.
	Tag() names.Tag

	// ModelAgentVersion returns the agent version the machine's model is currently
	// running or an error.
	ModelAgentVersion() (*version.Number, error)

	// MachineTag returns the identifier for the machine as the most specific type.
	MachineTag() names.MachineTag

	// Id returns the machine id.
	Id() string

	// String returns the machine as a string.
	String() string

	// Life returns the machine's lifecycle value.
	Life() params.Life

	// Refresh updates the cached local copy of the machine's data.
	Refresh() error

	// ProvisioningInfo returns the information required to provision a machine.
	ProvisioningInfo() (*params.ProvisioningInfo, error)

	// SetInstanceStatus sets the status for the provider instance.
	SetInstanceStatus(status status.Status, message string, data map[string]interface{}) error

	// InstanceStatus returns the status of the provider instance.
	InstanceStatus() (status.Status, string, error)

	// SetStatus sets the status of the machine.
	SetStatus(status status.Status, info string, data map[string]interface{}) error

	// Status returns the status of the machine.
	Status() (status.Status, string, error)

	// EnsureDead sets the machine lifecycle to Dead if it is Alive or
	// Dying. It does nothing otherwise.
	EnsureDead() error

	// Remove removes the machine from state. It will fail if the machine
	// is not Dead.
	Remove() error

	// MarkForRemoval indicates that the machine is ready to have any
	// provider-level resources cleaned up and be removed.
	MarkForRemoval() error

	// Series returns the operating system series running on the machine.
	//
	// NOTE: Unlike state.Machine.Series(), this method returns an error
	// as well, because it needs to do an API call.
	Series() (string, error)

	// AvailabilityZone returns an underlying provider's availability zone
	// for a machine.
	AvailabilityZone() (string, error)

	// DistributionGroup returns a slice of instance.Ids
	// that belong to the same distribution group as this
	// Machine. The provisioner may use this information
	// to distribute instances for high availability.
	DistributionGroup() ([]instance.Id, error)

	// SetInstanceInfo sets the provider specific instance id, nonce, metadata,
	// network config for this machine. Once set, the instance id cannot be changed.
	SetInstanceInfo(
		id instance.Id, nonce string, characteristics *instance.HardwareCharacteristics,
		networkConfig []params.NetworkConfig, volumes []params.Volume,
		volumeAttachments map[string]params.VolumeAttachmentInfo, charmProfiles []string,
	) error

	// InstanceId returns the provider specific instance id for the
	// machine or an CodeNotProvisioned error, if not set.
	InstanceId() (instance.Id, error)

	// KeepInstance returns the value of the keep-instance
	// for the machine.
	KeepInstance() (bool, error)

	// SetPassword sets the machine's password.
	SetPassword(password string) error

	// WatchContainers returns a StringsWatcher that notifies of changes
	// to the lifecycles of containers of the specified type on the machine.
	WatchContainers(ctype instance.ContainerType) (watcher.StringsWatcher, error)

	// WatchAllContainers returns a StringsWatcher that notifies of changes
	// to the lifecycles of all containers on the machine.
	WatchAllContainers() (watcher.StringsWatcher, error)

	// SetSupportedContainers updates the list of containers supported by this machine.
	SetSupportedContainers(containerTypes ...instance.ContainerType) error

	// SupportsNoContainers records the fact that this machine doesn't support any containers.
	SupportsNoContainers() error

	// WatchContainers returns a StringsWatcher that notifies of
	// changes to the upgrade charm profile charm url for all
	// containers of the specified type  on the machine.
	WatchContainersCharmProfiles(ctype instance.ContainerType) (watcher.StringsWatcher, error)
}

// Machine represents a juju machine as seen by the provisioner worker.
type Machine struct {
	tag  names.MachineTag
	life params.Life
	st   *State
}

// Tag implements MachineProvisioner.Tag.
func (m *Machine) Tag() names.Tag {
	return m.tag
}

// ModelAgentVersion implements MachineProvisioner.ModelAgentVersion.
func (m *Machine) ModelAgentVersion() (*version.Number, error) {
	mc, err := m.st.ModelConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}

	if v, ok := mc.AgentVersion(); ok {
		return &v, nil
	}

	return nil, errors.New("failed to get model's agent version.")
}

// MachineTag implements MachineProvisioner.MachineTag.
func (m *Machine) MachineTag() names.MachineTag {
	return m.tag
}

// Id implements MachineProvisioner.Id.
func (m *Machine) Id() string {
	return m.tag.Id()
}

// String implements MachineProvisioner.String.
func (m *Machine) String() string {
	return m.Id()
}

// Life implements MachineProvisioner..
func (m *Machine) Life() params.Life {
	return m.life
}

// Refresh implements MachineProvisioner.Refresh.
func (m *Machine) Refresh() error {
	life, err := m.st.machineLife(m.tag)
	if err != nil {
		return err
	}
	m.life = life
	return nil
}

// ProvisioningInfo implements MachineProvisioner.ProvisioningInfo.
func (m *Machine) ProvisioningInfo() (*params.ProvisioningInfo, error) {
	var results params.ProvisioningInfoResults
	args := params.Entities{Entities: []params.Entity{{m.tag.String()}}}
	err := m.st.facade.FacadeCall("ProvisioningInfo", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	return result.Result, nil
}

// SetInstanceStatus implements MachineProvisioner.SetInstanceStatus.
func (m *Machine) SetInstanceStatus(status status.Status, message string, data map[string]interface{}) error {
	var result params.ErrorResults
	args := params.SetStatus{Entities: []params.EntityStatusArgs{
		{Tag: m.tag.String(), Status: status.String(), Info: message, Data: data},
	}}
	err := m.st.facade.FacadeCall("SetInstanceStatus", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// InstanceStatus implements MachineProvisioner.InstanceStatus.
func (m *Machine) InstanceStatus() (status.Status, string, error) {
	var results params.StatusResults
	args := params.Entities{Entities: []params.Entity{
		{Tag: m.tag.String()},
	}}
	err := m.st.facade.FacadeCall("InstanceStatus", args, &results)
	if err != nil {
		return "", "", err
	}
	if len(results.Results) != 1 {
		return "", "", fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return "", "", result.Error
	}
	// TODO(perrito666) add status validation.
	return status.Status(result.Status), result.Info, nil
}

// SetStatus implements MachineProvisioner.SetStatus.
func (m *Machine) SetStatus(status status.Status, info string, data map[string]interface{}) error {
	var result params.ErrorResults
	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{Tag: m.tag.String(), Status: status.String(), Info: info, Data: data},
		},
	}
	err := m.st.facade.FacadeCall("SetStatus", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// Status implements MachineProvisioner.Status.
func (m *Machine) Status() (status.Status, string, error) {
	var results params.StatusResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag.String()}},
	}
	err := m.st.facade.FacadeCall("Status", args, &results)
	if err != nil {
		return "", "", err
	}
	if len(results.Results) != 1 {
		return "", "", fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return "", "", result.Error
	}
	// TODO(perrito666) add status validation.
	return status.Status(result.Status), result.Info, nil
}

// EnsureDead implements MachineProvisioner.EnsureDead.
func (m *Machine) EnsureDead() error {
	var result params.ErrorResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag.String()}},
	}
	err := m.st.facade.FacadeCall("EnsureDead", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// Remove implements MachineProvisioner.Remove.
func (m *Machine) Remove() error {
	var result params.ErrorResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag.String()}},
	}
	err := m.st.facade.FacadeCall("Remove", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// MarkForRemoval implements MachineProvisioner.MarkForRemoval.
func (m *Machine) MarkForRemoval() error {
	var result params.ErrorResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag.String()}},
	}
	err := m.st.facade.FacadeCall("MarkMachinesForRemoval", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// Series implements MachineProvisioner.Series.
func (m *Machine) Series() (string, error) {
	var results params.StringResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag.String()}},
	}
	err := m.st.facade.FacadeCall("Series", args, &results)
	if err != nil {
		return "", err
	}
	if len(results.Results) != 1 {
		return "", fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return "", result.Error
	}
	return result.Result, nil
}

// AvailabilityZone implements MachineProvisioner.AvailabilityZone.
func (m *Machine) AvailabilityZone() (string, error) {
	var results params.StringResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag.String()}},
	}
	err := m.st.facade.FacadeCall("AvailabilityZone", args, &results)
	if err != nil {
		return "", err
	}
	if len(results.Results) != 1 {
		return "", fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return "", result.Error
	}
	return result.Result, nil
}

// DistributionGroup implements MachineProvisioner.DistributionGroup.
func (m *Machine) DistributionGroup() ([]instance.Id, error) {
	var results params.DistributionGroupResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag.String()}},
	}
	err := m.st.facade.FacadeCall("DistributionGroup", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	return result.Result, nil
}

// SetInstanceInfo implements MachineProvisioner.SetInstanceInfo.
func (m *Machine) SetInstanceInfo(
	id instance.Id, nonce string, characteristics *instance.HardwareCharacteristics,
	networkConfig []params.NetworkConfig, volumes []params.Volume,
	volumeAttachments map[string]params.VolumeAttachmentInfo, charmProfiles []string,
) error {
	var result params.ErrorResults
	args := params.InstancesInfo{
		Machines: []params.InstanceInfo{{
			Tag:               m.tag.String(),
			InstanceId:        id,
			Nonce:             nonce,
			Characteristics:   characteristics,
			Volumes:           volumes,
			VolumeAttachments: volumeAttachments,
			NetworkConfig:     networkConfig,
			CharmProfiles:     charmProfiles,
		}},
	}
	err := m.st.facade.FacadeCall("SetInstanceInfo", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// InstanceId implements MachineProvisioner.InstanceId.
func (m *Machine) InstanceId() (instance.Id, error) {
	var results params.StringResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag.String()}},
	}
	err := m.st.facade.FacadeCall("InstanceId", args, &results)
	if err != nil {
		return "", err
	}
	if len(results.Results) != 1 {
		return "", fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return "", result.Error
	}
	return instance.Id(result.Result), nil
}

// KeepInstance implements MachineProvisioner.KeepInstance.
func (m *Machine) KeepInstance() (bool, error) {
	var results params.BoolResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag.String()}},
	}
	err := m.st.facade.FacadeCall("KeepInstance", args, &results)
	if err != nil {
		return false, err
	}
	if len(results.Results) != 1 {
		return false, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		if params.IsCodeNotSupported(err) {
			return false, errors.NewNotSupported(nil, "KeepInstance")
		}
		return false, result.Error
	}
	return result.Result, nil
}

// SetPassword implements MachineProvisioner.SetPassword.
func (m *Machine) SetPassword(password string) error {
	var result params.ErrorResults
	args := params.EntityPasswords{
		Changes: []params.EntityPassword{
			{Tag: m.tag.String(), Password: password},
		},
	}
	err := m.st.facade.FacadeCall("SetPasswords", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// WatchContainers implements MachineProvisioner.WatchContainers.
func (m *Machine) WatchContainers(ctype instance.ContainerType) (watcher.StringsWatcher, error) {
	if string(ctype) == "" {
		return nil, fmt.Errorf("container type must be specified")
	}
	supported := false
	for _, c := range instance.ContainerTypes {
		if ctype == c {
			supported = true
			break
		}
	}
	if !supported {
		return nil, fmt.Errorf("unsupported container type %q", ctype)
	}
	var results params.StringsWatchResults
	args := params.WatchContainers{
		Params: []params.WatchContainer{
			{MachineTag: m.tag.String(), ContainerType: string(ctype)},
		},
	}
	err := m.st.facade.FacadeCall("WatchContainers", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewStringsWatcher(m.st.facade.RawAPICaller(), result)
	return w, nil
}

// WatchAllContainers implements MachineProvisioner.WatchAllContainers.
func (m *Machine) WatchAllContainers() (watcher.StringsWatcher, error) {
	var results params.StringsWatchResults
	args := params.WatchContainers{
		Params: []params.WatchContainer{
			{MachineTag: m.tag.String()},
		},
	}
	err := m.st.facade.FacadeCall("WatchContainers", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewStringsWatcher(m.st.facade.RawAPICaller(), result)
	return w, nil
}

// WatchContainers implements MachineProvisioner.WatchContainersCharmProfiles.
func (m *Machine) WatchContainersCharmProfiles(ctype instance.ContainerType) (watcher.StringsWatcher, error) {
	if string(ctype) == "" {
		return nil, fmt.Errorf("container type must be specified")
	}
	supported := false
	for _, c := range instance.ContainerTypes {
		if ctype == c {
			supported = true
			break
		}
	}
	if !supported {
		return nil, fmt.Errorf("unsupported container type %q", ctype)
	}
	var results params.StringsWatchResults
	args := params.WatchContainers{
		Params: []params.WatchContainer{
			{MachineTag: m.tag.String(), ContainerType: string(ctype)},
		},
	}
	err := m.st.facade.FacadeCall("WatchContainersCharmProfiles", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewStringsWatcher(m.st.facade.RawAPICaller(), result)
	return w, nil
}

// SetSupportedContainers implements MachineProvisioner.SetSupportedContainers.
func (m *Machine) SetSupportedContainers(containerTypes ...instance.ContainerType) error {
	var results params.ErrorResults
	args := params.MachineContainersParams{
		Params: []params.MachineContainers{
			{MachineTag: m.tag.String(), ContainerTypes: containerTypes},
		},
	}
	err := m.st.facade.FacadeCall("SetSupportedContainers", args, &results)
	if err != nil {
		return err
	}
	if len(results.Results) != 1 {
		return fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	apiError := results.Results[0].Error
	if apiError != nil {
		return apiError
	}
	return nil
}

// SupportsNoContainers implements MachineProvisioner.SupportsNoContainers.
func (m *Machine) SupportsNoContainers() error {
	return m.SetSupportedContainers([]instance.ContainerType{}...)
}
