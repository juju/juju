// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/replicaset/v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/raftlease"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	raftleasestore "github.com/juju/juju/state/raftlease"
)

// StateBackend provides an interface for upgrading the global state database.
type StateBackend interface {
	ControllerUUID() (string, error)
	StateServingInfo() (controller.StateServingInfo, error)
	ControllerConfig() (controller.Config, error)
	LeaseNotifyTarget(raftleasestore.Logger) (raftlease.NotifyTarget, error)
	// Raft related functions
	ReplicaSetMembers() ([]replicaset.Member, error)
	LegacyLeases(time.Time) (map[lease.Key]lease.Info, error)

	// 2.9.x related functions
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

	// 3.0.x related functions
	MigrateCappedTxnsLogCollection() error
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

func (s stateBackend) ControllerUUID() (string, error) {
	systemState, err := s.pool.SystemState()
	return systemState.ControllerUUID(), err
}

func (s stateBackend) StateServingInfo() (controller.StateServingInfo, error) {
	systemState, err := s.pool.SystemState()
	if err != nil {
		return controller.StateServingInfo{}, errors.Trace(err)
	}
	ssi, errS := systemState.StateServingInfo()
	if errS != nil {
		return controller.StateServingInfo{}, errors.Trace(err)
	}
	return ssi, err
}

func (s stateBackend) StripLocalUserDomain() error {
	return state.StripLocalUserDomain(s.pool)
}

func (s stateBackend) RenameAddModelPermission() error {
	return state.RenameAddModelPermission(s.pool)
}

func (s stateBackend) AddMigrationAttempt() error {
	return state.AddMigrationAttempt(s.pool)
}

func (s stateBackend) AddLocalCharmSequences() error {
	return state.AddLocalCharmSequences(s.pool)
}

func (s stateBackend) UpdateLegacyLXDCloudCredentials(endpoint string, credential cloud.Credential) error {
	systemState, err := s.pool.SystemState()
	if err != nil {
		return errors.Trace(err)
	}
	return state.UpdateLegacyLXDCloudCredentials(systemState, endpoint, credential)
}

func (s stateBackend) UpgradeNoProxyDefaults() error {
	return state.UpgradeNoProxyDefaults(s.pool)
}

func (s stateBackend) AddNonDetachableStorageMachineId() error {
	return state.AddNonDetachableStorageMachineId(s.pool)
}

func (s stateBackend) AddControllerLogCollectionsSizeSettings() error {
	return state.AddControllerLogCollectionsSizeSettings(s.pool)
}

func (s stateBackend) AddStatusHistoryPruneSettings() error {
	return state.AddStatusHistoryPruneSettings(s.pool)
}

func (s stateBackend) AddActionPruneSettings() error {
	return state.AddActionPruneSettings(s.pool)
}

func (s stateBackend) ReplicaSetMembers() ([]replicaset.Member, error) {
	return state.ReplicaSetMembers(s.pool)
}

func (s stateBackend) ControllerConfig() (controller.Config, error) {
	systemState, err := s.pool.SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return systemState.ControllerConfig()
}

func (s stateBackend) LeaseNotifyTarget(logger raftleasestore.Logger) (raftlease.NotifyTarget, error) {
	systemState, err := s.pool.SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return systemState.LeaseNotifyTarget(logger), nil
}

func (s stateBackend) LegacyLeases(localTime time.Time) (map[lease.Key]lease.Info, error) {
	return state.LegacyLeases(s.pool, localTime)
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

func (s stateBackend) MigrateCappedTxnsLogCollection() error {
	return state.MigrateCappedTxnsLogCollection(s.pool)
}
