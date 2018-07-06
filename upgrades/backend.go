// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/replicaset"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
)

// StateBackend provides an interface for upgrading the global state database.
type StateBackend interface {
	ControllerUUID() string
	StateServingInfo() (state.StateServingInfo, error)

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
}

// Model is an interface providing access to the details of a model within the
// controller.
type Model interface {
	Config() (*config.Config, error)
	CloudSpec() (environs.CloudSpec, error)
}

// NewStateBackend returns a new StateBackend using a *state.State object.
func NewStateBackend(st *state.State) StateBackend {
	return stateBackend{st}
}

type stateBackend struct {
	st *state.State
}

func (s stateBackend) ControllerUUID() string {
	return s.st.ControllerUUID()
}

func (s stateBackend) StateServingInfo() (state.StateServingInfo, error) {
	return s.st.StateServingInfo()
}

func (s stateBackend) StripLocalUserDomain() error {
	return state.StripLocalUserDomain(s.st)
}

func (s stateBackend) RenameAddModelPermission() error {
	return state.RenameAddModelPermission(s.st)
}

func (s stateBackend) AddMigrationAttempt() error {
	return state.AddMigrationAttempt(s.st)
}

func (s stateBackend) AddLocalCharmSequences() error {
	return state.AddLocalCharmSequences(s.st)
}

func (s stateBackend) UpdateLegacyLXDCloudCredentials(endpoint string, credential cloud.Credential) error {
	return state.UpdateLegacyLXDCloudCredentials(s.st, endpoint, credential)
}

func (s stateBackend) UpgradeNoProxyDefaults() error {
	return state.UpgradeNoProxyDefaults(s.st)
}

func (s stateBackend) AddNonDetachableStorageMachineId() error {
	return state.AddNonDetachableStorageMachineId(s.st)
}

func (s stateBackend) RemoveNilValueApplicationSettings() error {
	return state.RemoveNilValueApplicationSettings(s.st)
}

func (s stateBackend) AddControllerLogCollectionsSizeSettings() error {
	return state.AddControllerLogCollectionsSizeSettings(s.st)
}

func (s stateBackend) AddStatusHistoryPruneSettings() error {
	return state.AddStatusHistoryPruneSettings(s.st)
}

func (s stateBackend) AddActionPruneSettings() error {
	return state.AddActionPruneSettings(s.st)
}

func (s stateBackend) AddUpdateStatusHookSettings() error {
	return state.AddUpdateStatusHookSettings(s.st)
}

func (s stateBackend) AddStorageInstanceConstraints() error {
	return state.AddStorageInstanceConstraints(s.st)
}

func (s stateBackend) SplitLogCollections() error {
	return state.SplitLogCollections(s.st)
}

func (s stateBackend) CorrectRelationUnitCounts() error {
	return state.CorrectRelationUnitCounts(s.st)
}

func (s stateBackend) AddModelEnvironVersion() error {
	return state.AddModelEnvironVersion(s.st)
}

func (s stateBackend) AddModelType() error {
	return state.AddModelType(s.st)
}

func (s stateBackend) MigrateLeasesToGlobalTime() error {
	return state.MigrateLeasesToGlobalTime(s.st)
}

func (s stateBackend) MoveOldAuditLog() error {
	return state.MoveOldAuditLog(s.st)
}

func (s stateBackend) AddRelationStatus() error {
	return state.AddRelationStatus(s.st)
}

func (s stateBackend) MoveMongoSpaceToHASpaceConfig() error {
	return state.MoveMongoSpaceToHASpaceConfig(s.st)
}

func (s stateBackend) CreateMissingApplicationConfig() error {
	return state.CreateMissingApplicationConfig(s.st)
}

func (s stateBackend) RemoveVotingMachineIds() error {
	return state.RemoveVotingMachineIds(s.st)
}

func (s stateBackend) AddCloudModelCounts() error {
	return state.AddCloudModelCounts(s.st)
}

func (s stateBackend) ReplicaSetMembers() ([]replicaset.Member, error) {
	return state.ReplicaSetMembers(s.st)
}

func (s stateBackend) MigrateStorageMachineIdFields() error {
	return state.MigrateStorageMachineIdFields(s.st)
}

type modelShim struct {
	st *state.State
	m  *state.Model
}

func (m *modelShim) Config() (*config.Config, error) {
	return m.m.Config()
}

func (m *modelShim) CloudSpec() (environs.CloudSpec, error) {
	cloudName := m.m.Cloud()
	regionName := m.m.CloudRegion()
	credentialTag, _ := m.m.CloudCredential()
	return stateenvirons.CloudSpec(m.st, cloudName, regionName, credentialTag)
}

func (s stateBackend) DeleteCloudImageMetadata() error {
	return state.DeleteCloudImageMetadata(s.st)
}

func (s stateBackend) EnsureContainerImageStreamDefault() error {
	return state.UpgradeContainerImageStreamDefault(s.st)
}

func (s stateBackend) RemoveContainerImageStreamFromNonModelSettings() error {
	return state.RemoveContainerImageStreamFromNonModelSettings(s.st)
}
