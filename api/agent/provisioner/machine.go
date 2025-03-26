// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/version"
	"github.com/juju/juju/rpc/params"
)

// MachineProvisioner defines what provisioner needs to provision a machine.
type MachineProvisioner interface {
	// Tag returns the machine's tag.
	Tag() names.Tag

	// ModelAgentVersion returns the agent version the machine's model is currently
	// running or an error.
	ModelAgentVersion(ctx context.Context) (*version.Number, error)

	// MachineTag returns the identifier for the machine as the most specific type.
	MachineTag() names.MachineTag

	// Id returns the machine id.
	Id() string

	// String returns the machine as a string.
	String() string

	// Life returns the machine's lifecycle value.
	Life() life.Value

	// Refresh updates the cached local copy of the machine's data.
	Refresh(context.Context) error

	// SetInstanceStatus sets the status for the provider instance.
	SetInstanceStatus(ctx context.Context, status status.Status, message string, data map[string]interface{}) error

	// InstanceStatus returns the status of the provider instance.
	InstanceStatus(ctx context.Context) (status.Status, string, error)

	// SetStatus sets the status of the machine.
	SetStatus(ctx context.Context, status status.Status, info string, data map[string]interface{}) error

	// Status returns the status of the machine.
	Status(ctx context.Context) (status.Status, string, error)

	// SetModificationStatus sets the status of the machine changes whilst it's
	// running. Example of this could be LXD profiles being applied.
	SetModificationStatus(ctx context.Context, status status.Status, message string, data map[string]interface{}) error

	// EnsureDead sets the machine lifecycle to Dead if it is Alive or
	// Dying. It does nothing otherwise.
	EnsureDead(ctx context.Context) error

	// Remove removes the machine from state. It will fail if the machine
	// is not Dead.
	Remove(ctx context.Context) error

	// MarkForRemoval indicates that the machine is ready to have any
	// provider-level resources cleaned up and be removed.
	MarkForRemoval(ctx context.Context) error

	// AvailabilityZone returns an underlying provider's availability zone
	// for a machine.
	AvailabilityZone(ctx context.Context) (string, error)

	// DistributionGroup returns a slice of instance.Ids
	// that belong to the same distribution group as this
	// Machine. The provisioner may use this information
	// to distribute instances for high availability.
	DistributionGroup(ctx context.Context) ([]instance.Id, error)

	// SetInstanceInfo sets the provider specific instance id, nonce, metadata,
	// network config for this machine. Once set, the instance id cannot be changed.
	SetInstanceInfo(
		ctx context.Context,
		id instance.Id, displayName string, nonce string, characteristics *instance.HardwareCharacteristics,
		networkConfig []params.NetworkConfig, volumes []params.Volume,
		volumeAttachments map[string]params.VolumeAttachmentInfo, charmProfiles []string,
	) error

	// InstanceId returns the provider specific instance id for the
	// machine or an CodeNotProvisioned error, if not set.
	InstanceId(ctx context.Context) (instance.Id, error)

	// KeepInstance returns the value of the keep-instance
	// for the machine.
	KeepInstance(ctx context.Context) (bool, error)

	// SetPassword sets the machine's password.
	SetPassword(ctx context.Context, password string) error

	// WatchContainers returns a StringsWatcher that notifies of changes
	// to the lifecycles of containers of the specified type on the machine.
	WatchContainers(ctx context.Context, ctype instance.ContainerType) (watcher.StringsWatcher, error)

	// SetSupportedContainers updates the list of containers supported by this machine.
	SetSupportedContainers(ctx context.Context, containerTypes ...instance.ContainerType) error

	// SupportsNoContainers records the fact that this machine doesn't support any containers.
	SupportsNoContainers(ctx context.Context) error

	// SupportedContainers returns a list of containers supported by this machine.
	SupportedContainers(ctx context.Context) ([]instance.ContainerType, bool, error)

	// SetCharmProfiles records the given slice of charm profile names.
	SetCharmProfiles(context.Context, []string) error
}

// Machine represents a juju machine as seen by the provisioner worker.
type Machine struct {
	tag  names.MachineTag
	life life.Value
	st   *Client
}

// Tag implements MachineProvisioner.Tag.
func (m *Machine) Tag() names.Tag {
	return m.tag
}

// ModelAgentVersion implements MachineProvisioner.ModelAgentVersion.
func (m *Machine) ModelAgentVersion(ctx context.Context) (*version.Number, error) {
	mc, err := m.st.ModelConfig(ctx)
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
func (m *Machine) Life() life.Value {
	return m.life
}

// Refresh implements MachineProvisioner.Refresh.
func (m *Machine) Refresh(ctx context.Context) error {
	life, err := m.st.machineLife(ctx, m.tag)
	if err != nil {
		return err
	}
	m.life = life
	return nil
}

// SetInstanceStatus implements MachineProvisioner.SetInstanceStatus.
func (m *Machine) SetInstanceStatus(ctx context.Context, status status.Status, message string, data map[string]interface{}) error {
	var result params.ErrorResults
	args := params.SetStatus{Entities: []params.EntityStatusArgs{
		{Tag: m.tag.String(), Status: status.String(), Info: message, Data: data},
	}}
	err := m.st.facade.FacadeCall(ctx, "SetInstanceStatus", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// InstanceStatus implements MachineProvisioner.InstanceStatus.
func (m *Machine) InstanceStatus(ctx context.Context) (status.Status, string, error) {
	var results params.StatusResults
	args := params.Entities{Entities: []params.Entity{
		{Tag: m.tag.String()},
	}}
	err := m.st.facade.FacadeCall(ctx, "InstanceStatus", args, &results)
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
func (m *Machine) SetStatus(ctx context.Context, status status.Status, info string, data map[string]interface{}) error {
	var result params.ErrorResults
	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{Tag: m.tag.String(), Status: status.String(), Info: info, Data: data},
		},
	}
	err := m.st.facade.FacadeCall(ctx, "SetStatus", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// Status implements MachineProvisioner.Status.
func (m *Machine) Status(ctx context.Context) (status.Status, string, error) {
	var results params.StatusResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag.String()}},
	}
	err := m.st.facade.FacadeCall(ctx, "Status", args, &results)
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

// SetModificationStatus implements MachineProvisioner.SetModificationStatus.
func (m *Machine) SetModificationStatus(ctx context.Context, status status.Status, info string, data map[string]interface{}) error {
	var result params.ErrorResults
	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{Tag: m.tag.String(), Status: status.String(), Info: info, Data: data},
		},
	}
	err := m.st.facade.FacadeCall(ctx, "SetModificationStatus", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// EnsureDead implements MachineProvisioner.EnsureDead.
func (m *Machine) EnsureDead(ctx context.Context) error {
	var result params.ErrorResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag.String()}},
	}
	err := m.st.facade.FacadeCall(ctx, "EnsureDead", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// Remove implements MachineProvisioner.Remove.
func (m *Machine) Remove(ctx context.Context) error {
	var result params.ErrorResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag.String()}},
	}
	err := m.st.facade.FacadeCall(ctx, "Remove", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// MarkForRemoval implements MachineProvisioner.MarkForRemoval.
func (m *Machine) MarkForRemoval(ctx context.Context) error {
	var result params.ErrorResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag.String()}},
	}
	err := m.st.facade.FacadeCall(ctx, "MarkMachinesForRemoval", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// AvailabilityZone implements MachineProvisioner.AvailabilityZone.
func (m *Machine) AvailabilityZone(ctx context.Context) (string, error) {
	var results params.StringResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag.String()}},
	}
	err := m.st.facade.FacadeCall(ctx, "AvailabilityZone", args, &results)
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
func (m *Machine) DistributionGroup(ctx context.Context) ([]instance.Id, error) {
	var results params.DistributionGroupResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag.String()}},
	}
	err := m.st.facade.FacadeCall(ctx, "DistributionGroup", args, &results)
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
	ctx context.Context,
	id instance.Id, displayName string, nonce string, characteristics *instance.HardwareCharacteristics,
	networkConfig []params.NetworkConfig, volumes []params.Volume,
	volumeAttachments map[string]params.VolumeAttachmentInfo, charmProfiles []string,
) error {
	var result params.ErrorResults
	args := params.InstancesInfo{
		Machines: []params.InstanceInfo{{
			Tag:               m.tag.String(),
			InstanceId:        id,
			DisplayName:       displayName,
			Nonce:             nonce,
			Characteristics:   characteristics,
			Volumes:           volumes,
			VolumeAttachments: volumeAttachments,
			NetworkConfig:     networkConfig,
			CharmProfiles:     charmProfiles,
		}},
	}
	err := m.st.facade.FacadeCall(ctx, "SetInstanceInfo", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// InstanceId implements MachineProvisioner.InstanceId.
func (m *Machine) InstanceId(ctx context.Context) (instance.Id, error) {
	var results params.StringResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag.String()}},
	}
	err := m.st.facade.FacadeCall(ctx, "InstanceId", args, &results)
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
func (m *Machine) KeepInstance(ctx context.Context) (bool, error) {
	var results params.BoolResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag.String()}},
	}
	err := m.st.facade.FacadeCall(ctx, "KeepInstance", args, &results)
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
func (m *Machine) SetPassword(ctx context.Context, password string) error {
	var result params.ErrorResults
	args := params.EntityPasswords{
		Changes: []params.EntityPassword{
			{Tag: m.tag.String(), Password: password},
		},
	}
	err := m.st.facade.FacadeCall(ctx, "SetPasswords", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// WatchContainers implements MachineProvisioner.WatchContainers.
func (m *Machine) WatchContainers(ctx context.Context, ctype instance.ContainerType) (watcher.StringsWatcher, error) {
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
	err := m.st.facade.FacadeCall(ctx, "WatchContainers", args, &results)
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
func (m *Machine) SetSupportedContainers(ctx context.Context, containerTypes ...instance.ContainerType) error {
	var results params.ErrorResults
	args := params.MachineContainersParams{
		Params: []params.MachineContainers{
			{MachineTag: m.tag.String(), ContainerTypes: containerTypes},
		},
	}
	err := m.st.facade.FacadeCall(ctx, "SetSupportedContainers", args, &results)
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
func (m *Machine) SupportsNoContainers(ctx context.Context) error {
	return m.SetSupportedContainers(ctx, []instance.ContainerType{}...)
}

// SupportedContainers implements MachineProvisioner.SupportedContainers.
func (m *Machine) SupportedContainers(ctx context.Context) ([]instance.ContainerType, bool, error) {
	var results params.MachineContainerResults
	args := params.Entities{
		Entities: []params.Entity{
			{Tag: m.tag.String()},
		},
	}
	err := m.st.facade.FacadeCall(ctx, "SupportedContainers", args, &results)
	if err != nil {
		return nil, false, err
	}
	if len(results.Results) != 1 {
		return nil, false, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	apiError := results.Results[0].Error
	if apiError != nil {
		return nil, false, apiError
	}
	result := results.Results[0]
	return result.ContainerTypes, result.Determined, nil
}

// SetCharmProfiles implements MachineProvisioner.SetCharmProfiles.
func (m *Machine) SetCharmProfiles(ctx context.Context, profiles []string) error {
	var results params.ErrorResults
	args := params.SetProfileArgs{
		Args: []params.SetProfileArg{
			{
				Entity:   params.Entity{Tag: m.tag.String()},
				Profiles: profiles,
			},
		},
	}
	err := m.st.facade.FacadeCall(ctx, "SetCharmProfiles", args, &results)
	if err != nil {
		return err
	}
	if len(results.Results) != 1 {
		return fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return result.Error
	}
	return nil
}
