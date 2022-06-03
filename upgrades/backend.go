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
	ChangeSubnetSpaceNameToSpaceID() error
	AddSubnetIdToSubnetDocs() error
	ReplacePortsDocSubnetIDCIDR() error
	EnsureRelationApplicationSettings() error
	ConvertAddressSpaceIDs() error
	ReplaceSpaceNameWithIDEndpointBindings() error
	EnsureDefaultSpaceSetting() error
	RemoveControllerConfigMaxLogAgeAndSize() error
	IncrementTasksSequence() error
	AddMachineIDToSubordinates() error
	AddOriginToIPAddresses() error
	DropPresenceDatabase() error
	DropLeasesCollection() error
	RemoveUnsupportedLinkLayer() error
	AddBakeryConfig() error
	ReplaceNeverSetWithUnset() error
	ResetDefaultRelationLimitInCharmMetadata() error
	RollUpAndConvertOpenedPortDocuments() error
	AddCharmHubToModelConfig() error
	AddCharmOriginToApplications() error
	ExposeWildcardEndpointForExposedApplications() error
	RemoveLinkLayerDevicesRefsCollection() error
	RemoveUnusedLinkLayerDeviceProviderIDs() error
	TranslateK8sServiceTypes() error
	UpdateKubernetesCloudCredentials() error
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

func (s stateBackend) ChangeSubnetSpaceNameToSpaceID() error {
	return state.ChangeSubnetSpaceNameToSpaceID(s.pool)
}

func (s stateBackend) AddSubnetIdToSubnetDocs() error {
	return state.AddSubnetIdToSubnetDocs(s.pool)
}

func (s stateBackend) ReplacePortsDocSubnetIDCIDR() error {
	return state.ReplacePortsDocSubnetIDCIDR(s.pool)
}

func (s stateBackend) EnsureRelationApplicationSettings() error {
	return state.EnsureRelationApplicationSettings(s.pool)
}

func (s stateBackend) ConvertAddressSpaceIDs() error {
	return state.ConvertAddressSpaceIDs(s.pool)
}

func (s stateBackend) ReplaceSpaceNameWithIDEndpointBindings() error {
	return state.ReplaceSpaceNameWithIDEndpointBindings(s.pool)
}

func (s stateBackend) EnsureDefaultSpaceSetting() error {
	return state.EnsureDefaultSpaceSetting(s.pool)
}
func (s stateBackend) RemoveControllerConfigMaxLogAgeAndSize() error {
	return state.RemoveControllerConfigMaxLogAgeAndSize(s.pool)
}

func (s stateBackend) IncrementTasksSequence() error {
	return state.IncrementTasksSequence(s.pool)
}

func (s stateBackend) AddMachineIDToSubordinates() error {
	return state.AddMachineIDToSubordinates(s.pool)
}

func (s stateBackend) AddOriginToIPAddresses() error {
	return state.AddOriginToIPAddresses(s.pool)
}

func (s stateBackend) DropPresenceDatabase() error {
	return state.DropPresenceDatabase(s.pool)
}

func (s stateBackend) DropLeasesCollection() error {
	return state.DropLeasesCollection(s.pool)
}

func (s stateBackend) RemoveUnsupportedLinkLayer() error {
	return state.RemoveUnsupportedLinkLayer(s.pool)
}

func (s stateBackend) AddBakeryConfig() error {
	return state.AddBakeryConfig(s.pool)
}

func (s stateBackend) ReplaceNeverSetWithUnset() error {
	return state.ReplaceNeverSetWithUnset(s.pool)
}

func (s stateBackend) ResetDefaultRelationLimitInCharmMetadata() error {
	return state.ResetDefaultRelationLimitInCharmMetadata(s.pool)
}

func (s stateBackend) AddCharmHubToModelConfig() error {
	return state.AddCharmHubToModelConfig(s.pool)
}

func (s stateBackend) RollUpAndConvertOpenedPortDocuments() error {
	return state.RollUpAndConvertOpenedPortDocuments(s.pool)
}

func (s stateBackend) AddCharmOriginToApplications() error {
	return state.AddCharmOriginToApplications(s.pool)
}

func (s stateBackend) ExposeWildcardEndpointForExposedApplications() error {
	return state.ExposeWildcardEndpointForExposedApplications(s.pool)
}

func (s stateBackend) RemoveLinkLayerDevicesRefsCollection() error {
	return state.RemoveLinkLayerDevicesRefsCollection(s.pool)
}

func (s stateBackend) RemoveUnusedLinkLayerDeviceProviderIDs() error {
	return state.RemoveUnusedLinkLayerDeviceProviderIDs(s.pool)
}

func (s stateBackend) TranslateK8sServiceTypes() error {
	return state.TranslateK8sServiceTypes(s.pool)
}

func (s stateBackend) UpdateKubernetesCloudCredentials() error {
	systemState, err := s.pool.SystemState()
	if err != nil {
		return errors.Trace(err)
	}
	return state.UpdateLegacyKubernetesCloudCredentials(systemState)
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
