// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

// StateBackend provides an interface for upgrading the global state database.
type StateBackend interface {
	RemoveUnusedLinkLayerDeviceProviderIDs() error
	UpdateDHCPAddressConfigs() error
	KubernetesInClusterCredentialSpec() (environscloudspec.CloudSpec, *config.Config, string, error)
	AddSpawnedTaskCountToOperations() error
	TransformEmptyManifestsToNil() error
	EnsureCharmOriginRisk() error
	RemoveOrphanedCrossModelProxies() error
	DropLegacyAssumesSectionsFromCharmMetadata() error
	MigrateLegacyCrossModelTokens() error
	CleanupDeadAssignUnits() error
	RemoveOrphanedLinkLayerDevices() error
	UpdateExternalControllerInfo() error
	RemoveInvalidCharmPlaceholders() error
	SetContainerAddressOriginToMachine() error
	UpdateCharmOriginAfterSetSeries() error
	UpdateOperationWithEnqueuingErrors() error
	RemoveLocalCharmOriginChannels() error
	FixCharmhubLastPolltime() error
	RemoveUseFloatingIPConfigFalse() error
	CharmOriginChannelMustHaveTrack() error
	RemoveDefaultSeriesFromModelConfig() error
}

// Model is an interface providing access to the details of a model within the
// controller.
type Model interface {
	Config() (*config.Config, error)
	CloudSpec() (environscloudspec.CloudSpec, error)
}

// NewStateBackend returns a new StateBackend using a *state.StatePool object.
func NewStateBackend(pool *state.StatePool) StateBackend {
	return stateBackend{pool}
}

type stateBackend struct {
	pool *state.StatePool
}

func (s stateBackend) RemoveUnusedLinkLayerDeviceProviderIDs() error {
	return state.RemoveUnusedLinkLayerDeviceProviderIDs(s.pool)
}

func (s stateBackend) UpdateDHCPAddressConfigs() error {
	return state.UpdateDHCPAddressConfigs(s.pool)
}

func (s stateBackend) KubernetesInClusterCredentialSpec() (
	environscloudspec.CloudSpec, *config.Config, string, error) {
	return state.KubernetesInClusterCredentialSpec(s.pool)
}

func (s stateBackend) AddSpawnedTaskCountToOperations() error {
	return state.AddSpawnedTaskCountToOperations(s.pool)
}

func (s stateBackend) TransformEmptyManifestsToNil() error {
	return state.TransformEmptyManifestsToNil(s.pool)
}

func (s stateBackend) EnsureCharmOriginRisk() error {
	return state.EnsureCharmOriginRisk(s.pool)
}

func (s stateBackend) RemoveOrphanedCrossModelProxies() error {
	return state.RemoveOrphanedCrossModelProxies(s.pool)
}

func (s stateBackend) DropLegacyAssumesSectionsFromCharmMetadata() error {
	return state.DropLegacyAssumesSectionsFromCharmMetadata(s.pool)
}

func (s stateBackend) MigrateLegacyCrossModelTokens() error {
	return state.MigrateLegacyCrossModelTokens(s.pool)
}

func (s stateBackend) CleanupDeadAssignUnits() error {
	return state.CleanupDeadAssignUnits(s.pool)
}

func (s stateBackend) RemoveOrphanedLinkLayerDevices() error {
	return state.RemoveOrphanedLinkLayerDevices(s.pool)
}

func (s stateBackend) UpdateExternalControllerInfo() error {
	return state.UpdateExternalControllerInfo(s.pool)
}

func (s stateBackend) RemoveInvalidCharmPlaceholders() error {
	return state.RemoveInvalidCharmPlaceholders(s.pool)
}

func (s stateBackend) SetContainerAddressOriginToMachine() error {
	return state.SetContainerAddressOriginToMachine(s.pool)
}

func (s stateBackend) UpdateCharmOriginAfterSetSeries() error {
	return state.UpdateCharmOriginAfterSetSeries(s.pool)
}

func (s stateBackend) UpdateOperationWithEnqueuingErrors() error {
	return state.UpdateOperationWithEnqueuingErrors(s.pool)
}

func (s stateBackend) RemoveLocalCharmOriginChannels() error {
	return state.RemoveLocalCharmOriginChannels(s.pool)
}

func (s stateBackend) FixCharmhubLastPolltime() error {
	return state.FixCharmhubLastPolltime(s.pool)
}

func (s stateBackend) RemoveUseFloatingIPConfigFalse() error {
	return state.RemoveUseFloatingIPConfigFalse(s.pool)
}

func (s stateBackend) CharmOriginChannelMustHaveTrack() error {
	return state.CharmOriginChannelMustHaveTrack(s.pool)
}

func (s stateBackend) RemoveDefaultSeriesFromModelConfig() error {
	return state.RemoveDefaultSeriesFromModelConfig(s.pool)
}
