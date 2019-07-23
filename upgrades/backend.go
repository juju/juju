// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"io"
	"time"

	"github.com/juju/replicaset"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/raftlease"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	raftleasestore "github.com/juju/juju/state/raftlease"
)

// StateBackend provides an interface for upgrading the global state database.
type StateBackend interface {
	ControllerUUID() string
	StateServingInfo() (state.StateServingInfo, error)
	ControllerConfig() (controller.Config, error)
	LeaseNotifyTarget(io.Writer, raftleasestore.Logger) raftlease.NotifyTarget

	StripLocalUserDomain() error
	RenameAddModelPermission() error
	AddMigrationAttempt() error
	AddLocalCharmSequences() error
	UpdateLegacyLXDCloudCredentials(string, cloud.Credential) error
	UpgradeNoProxyDefaults() error
	AddNonDetachableStorageMachineId() error
	RemoveNilValueApplicationSettings() error
	AddControllerLogCollectionsSizeSettings() error
	AddStatusHistoryPruneSettings() error
	AddActionPruneSettings() error
	AddStorageInstanceConstraints() error
	SplitLogCollections() error
	AddUpdateStatusHookSettings() error
	CorrectRelationUnitCounts() error
	AddModelEnvironVersion() error
	AddModelType() error
	MigrateLeasesToGlobalTime() error
	MoveOldAuditLog() error
	AddRelationStatus() error
	DeleteCloudImageMetadata() error
	EnsureContainerImageStreamDefault() error
	RemoveContainerImageStreamFromNonModelSettings() error
	MoveMongoSpaceToHASpaceConfig() error
	CreateMissingApplicationConfig() error
	RemoveVotingMachineIds() error
	AddCloudModelCounts() error
	ReplicaSetMembers() ([]replicaset.Member, error)
	MigrateStorageMachineIdFields() error
	MigrateAddModelPermissions() error
	LegacyLeases(time.Time) (map[lease.Key]lease.Info, error)
	SetEnableDiskUUIDOnVsphere() error
	UpdateInheritedControllerConfig() error
	UpdateKubernetesStorageConfig() error
	EnsureDefaultModificationStatus() error
	EnsureApplicationDeviceConstraints() error
	RemoveInstanceCharmProfileDataCollection() error
	UpdateK8sModelNameIndex() error
	AddModelLogsSize() error
	AddControllerNodeDocs() error
	AddSpaceIdToSpaceDocs() error
	ChangeSubnetAZtoSlice() error
}

// Model is an interface providing access to the details of a model within the
// controller.
type Model interface {
	Config() (*config.Config, error)
	CloudSpec() (environs.CloudSpec, error)
}

// NewStateBackend returns a new StateBackend using a *state.StatePool object.
func NewStateBackend(pool *state.StatePool) StateBackend {
	return stateBackend{pool}
}

type stateBackend struct {
	pool *state.StatePool
}

func (s stateBackend) ControllerUUID() string {
	return s.pool.SystemState().ControllerUUID()
}

func (s stateBackend) StateServingInfo() (state.StateServingInfo, error) {
	return s.pool.SystemState().StateServingInfo()
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
	return state.UpdateLegacyLXDCloudCredentials(s.pool.SystemState(), endpoint, credential)
}

func (s stateBackend) UpgradeNoProxyDefaults() error {
	return state.UpgradeNoProxyDefaults(s.pool)
}

func (s stateBackend) AddNonDetachableStorageMachineId() error {
	return state.AddNonDetachableStorageMachineId(s.pool)
}

func (s stateBackend) RemoveNilValueApplicationSettings() error {
	return state.RemoveNilValueApplicationSettings(s.pool)
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

func (s stateBackend) AddUpdateStatusHookSettings() error {
	return state.AddUpdateStatusHookSettings(s.pool)
}

func (s stateBackend) AddStorageInstanceConstraints() error {
	return state.AddStorageInstanceConstraints(s.pool)
}

func (s stateBackend) SplitLogCollections() error {
	return state.SplitLogCollections(s.pool)
}

func (s stateBackend) CorrectRelationUnitCounts() error {
	return state.CorrectRelationUnitCounts(s.pool)
}

func (s stateBackend) AddModelEnvironVersion() error {
	return state.AddModelEnvironVersion(s.pool)
}

func (s stateBackend) AddModelType() error {
	return state.AddModelType(s.pool)
}

func (s stateBackend) MigrateLeasesToGlobalTime() error {
	return state.MigrateLeasesToGlobalTime(s.pool)
}

func (s stateBackend) MoveOldAuditLog() error {
	return state.MoveOldAuditLog(s.pool)
}

func (s stateBackend) AddRelationStatus() error {
	return state.AddRelationStatus(s.pool)
}

func (s stateBackend) MoveMongoSpaceToHASpaceConfig() error {
	return state.MoveMongoSpaceToHASpaceConfig(s.pool)
}

func (s stateBackend) CreateMissingApplicationConfig() error {
	return state.CreateMissingApplicationConfig(s.pool)
}

func (s stateBackend) RemoveVotingMachineIds() error {
	return state.RemoveVotingMachineIds(s.pool)
}

func (s stateBackend) AddCloudModelCounts() error {
	return state.AddCloudModelCounts(s.pool)
}

func (s stateBackend) ReplicaSetMembers() ([]replicaset.Member, error) {
	return state.ReplicaSetMembers(s.pool)
}

func (s stateBackend) MigrateStorageMachineIdFields() error {
	return state.MigrateStorageMachineIdFields(s.pool)
}

func (s stateBackend) MigrateAddModelPermissions() error {
	return state.MigrateAddModelPermissions(s.pool)
}

func (s stateBackend) DeleteCloudImageMetadata() error {
	return state.DeleteCloudImageMetadata(s.pool)
}

func (s stateBackend) EnsureContainerImageStreamDefault() error {
	return state.UpgradeContainerImageStreamDefault(s.pool)
}

func (s stateBackend) RemoveContainerImageStreamFromNonModelSettings() error {
	return state.RemoveContainerImageStreamFromNonModelSettings(s.pool)
}

func (s stateBackend) ControllerConfig() (controller.Config, error) {
	return s.pool.SystemState().ControllerConfig()
}

func (s stateBackend) LeaseNotifyTarget(w io.Writer, logger raftleasestore.Logger) raftlease.NotifyTarget {
	return s.pool.SystemState().LeaseNotifyTarget(w, logger)
}

func (s stateBackend) LegacyLeases(localTime time.Time) (map[lease.Key]lease.Info, error) {
	return state.LegacyLeases(s.pool, localTime)
}

func (s stateBackend) SetEnableDiskUUIDOnVsphere() error {
	return state.SetEnableDiskUUIDOnVsphere(s.pool)
}

func (s stateBackend) UpdateInheritedControllerConfig() error {
	return state.UpdateInheritedControllerConfig(s.pool)
}

func (s stateBackend) UpdateKubernetesStorageConfig() error {
	return state.UpdateKubernetesStorageConfig(s.pool)
}

func (s stateBackend) EnsureDefaultModificationStatus() error {
	return state.EnsureDefaultModificationStatus(s.pool)
}

func (s stateBackend) EnsureApplicationDeviceConstraints() error {
	return state.EnsureApplicationDeviceConstraints(s.pool)
}

func (s stateBackend) RemoveInstanceCharmProfileDataCollection() error {
	return state.RemoveInstanceCharmProfileDataCollection(s.pool)
}

func (s stateBackend) UpdateK8sModelNameIndex() error {
	return state.UpdateK8sModelNameIndex(s.pool)
}

func (s stateBackend) AddModelLogsSize() error {
	return state.AddModelLogsSize(s.pool)
}

func (s stateBackend) AddControllerNodeDocs() error {
	return state.AddControllerNodeDocs(s.pool)
}

func (s stateBackend) AddSpaceIdToSpaceDocs() error {
	return state.AddSpaceIdToSpaceDocs(s.pool)
}

func (s stateBackend) ChangeSubnetAZtoSlice() error {
	return state.ChangeSubnetAZtoSlice(s.pool)
}
