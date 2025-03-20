// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/charm/v12"
	"github.com/juju/collections/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

type MigrationSuite struct{}

var _ = gc.Suite(&MigrationSuite{})

func (s *MigrationSuite) TestKnownCollections(c *gc.C) {
	completedCollections := set.NewStrings(
		annotationsC,
		blocksC,
		cloudimagemetadataC,
		constraintsC,
		modelsC,
		modelUsersC,
		modelUserLastConnectionC,
		permissionsC,
		settingsC,
		generationsC,
		sequenceC,
		sshHostKeysC,
		statusesC,
		statusesHistoryC,
		virtualHostKeysC,

		// machine
		instanceDataC,
		machineUpgradeSeriesLocksC,
		machinesC,
		openedPortsC,

		// application / unit
		applicationsC,
		unitsC,
		meterStatusC, // red / green status for metrics of units
		payloadsC,
		resourcesC,

		// relation
		relationsC,
		relationScopesC,

		// networking
		endpointBindingsC,
		ipAddressesC,
		spacesC,
		linkLayerDevicesC,
		subnetsC,

		// storage
		blockDevicesC,

		// cloudimagemetadata
		cloudimagemetadataC,

		// actions
		actionsC,
		operationsC,

		// storage
		filesystemsC,
		filesystemAttachmentsC,
		storageAttachmentsC,
		storageConstraintsC,
		storageInstancesC,
		volumesC,
		volumeAttachmentsC,

		// caas
		podSpecsC,
		cloudContainersC,
		cloudServicesC,
		deviceConstraintsC,

		// crossmodelrelations
		remoteApplicationsC,
		applicationOffersC,
		offerConnectionsC,
		relationNetworksC,
		remoteEntitiesC,
		externalControllersC,

		// secrets
		secretMetadataC,
		secretRevisionsC,
		secretRotateC,
		secretConsumersC,
		secretRemoteConsumersC,
		secretPermissionsC,
	)

	ignoredCollections := set.NewStrings(
		// Precheck ensures that there are no cleanup docs or pending
		// machine removals.
		cleanupsC,
		machineRemovalsC,
		// The autocert cache is non-critical. After migration
		// you'll just need to acquire new certificates.
		autocertCacheC,
		// We don't export the controller model at this stage.
		controllersC,
		controllerNodesC,
		// Clouds aren't migrated. They must exist in the
		// target controller already.
		cloudsC,
		// Cloud credentials aren't migrated. They must exist in the
		// target controller already.
		cloudCredentialsC,
		// Users aren't migrated.
		usersC,
		userLastLoginC,
		// Controller users contain extra data about users therefore
		// are not migrated either.
		controllerUsersC,
		// userenvnameC is just to provide a unique key constraint.
		usermodelnameC,
		// Metrics aren't migrated.
		metricsC,
		// reference counts are implementation details that should be
		// reconstructed on the other side.
		refcountsC,
		globalRefcountsC,
		// upgradeInfoC is used to coordinate upgrades and schema migrations,
		// and aren't needed for model migrations.
		upgradeInfoC,
		// Not exported, but the tools will possibly need to be either bundled
		// with the representation or sent separately.
		toolsmetadataC,
		// Bakery storage items are non-critical. We store root keys for
		// temporary credentials in there; after migration you'll just have
		// to log back in.
		bakeryStorageItemsC,
		// Transaction stuff.
		"txns",
		"txns.log",
		"sstxns.log",

		// We don't import any of the migration collections.
		migrationsC,
		migrationsStatusC,
		migrationsStatusMessageC,
		migrationsActiveC,
		migrationsMinionSyncC,

		// The container ref document is primarily there to keep track
		// of a particular machine's containers. The migration format
		// uses object containment for this purpose.
		containerRefsC,
		// The min units collection is only used to trigger a watcher
		// in order to have the application add or remove units if the minimum
		// number of units is changed. The Application doc has all we need
		// for migratino.
		minUnitsC,
		// This is a transitory collection of units that need to be assigned
		// to machines.
		assignUnitC,

		// The model entity references collection will be repopulated
		// after importing the model. It does not need to be migrated
		// separately.
		modelEntityRefsC,

		// This is marked as deprecated, and should probably be removed.
		actionresultsC,

		// These are recreated whilst migrating other network entities.
		providerIDsC,

		// Recreated whilst migrating actions.
		actionNotificationsC,

		// Global settings store controller specific configuration settings
		// and are not to be migrated.
		globalSettingsC,

		// There is a precheck to ensure that there are no pending reboots
		// for the model being migrated, and as such, there is no need to
		// migrate that information.
		rebootC,

		// Charms are added into the migrated model during the binary transfer
		// phase after the initial model migration.
		charmsC,

		// Metrics manager maintains controller specific state relating to
		// the store and forward of charm metrics. Nothing to migrate here.
		metricsManagerC,

		// The global clock is not migrated; each controller has its own
		// independent global clock.
		globalClockC,

		// Volume attachment plans are ignored if missing. A missing collection
		// simply defaults to the old code path.
		volumeAttachmentPlanC,

		// Resources are transferred separately
		"storedResources",

		// Unit state entries will be automatically created when the
		// operator framework code mutates the state for the charm
		// running within a unit. This is a new feature that is not
		// backwards compatible with older controllers.
		unitStatesC,

		// Secret backends are per controller.
		secretBackendsC,
		secretBackendsRotateC,

		// sshConnRequestsC is a new collection and doesn't need to be
		// migrated.
		sshConnRequestsC,
	)

	// THIS SET WILL BE REMOVED WHEN MIGRATIONS ARE COMPLETE
	todoCollections := set.NewStrings(
		dockerResourcesC,
	)

	modelCollections := set.NewStrings()
	for name := range allCollections() {
		modelCollections.Add(name)
	}

	known := completedCollections.Union(ignoredCollections)

	remainder := modelCollections.Difference(known)
	remainder = remainder.Difference(todoCollections)

	// If this test fails, it means that a new collection has been added
	// but migrations for it has not been done. This is a Bad Thing™.
	// Beware, if your collection is something controller-related it might
	// not need migration (such as Users or ControllerUsers) in that
	// case they only need to be accounted for among the ignored collections.
	c.Assert(remainder, gc.HasLen, 0)
}

func (s *MigrationSuite) TestModelDocFields(c *gc.C) {
	fields := set.NewStrings(
		// UUID and Name are constructed from the model config.
		"UUID",
		"Name",
		// Life will always be alive, or we won't be migrating.
		"Life",
		// ForceDestroyed is only relevant for models that are being
		// removed.
		"ForceDestroyed",
		// DestroyTimeout is only relevant for models that are being
		// removed.
		"DestroyTimeout",
		// ControllerUUID is recreated when the new model is created
		// in the new controller (yay name changes).
		"ControllerUUID",

		"Type",
		"MigrationMode",
		"Owner",
		"Cloud",
		"CloudRegion",
		"CloudCredential",
		"LatestAvailableTools",
		"SLA",
		"MeterStatus",
		"EnvironVersion",
		"PasswordHash",
	)
	s.AssertExportedFields(c, modelDoc{}, fields)
}

func (s *MigrationSuite) TestUserAccessDocFields(c *gc.C) {
	fields := set.NewStrings(
		// ID is the same as UserName (but lowercased)
		"ID",
		// ObjectUUID shouldn't be exported, and is inherited
		// from the model definition.
		"ObjectUUID",
		// Tracked fields:
		"UserName",
		"DisplayName",
		"CreatedBy",
		"DateCreated",
	)
	s.AssertExportedFields(c, userAccessDoc{}, fields)
}

func (s *MigrationSuite) TestPermissionDocFields(c *gc.C) {
	fields := set.NewStrings(
		"ID",
		"ObjectGlobalKey",
		"SubjectGlobalKey",
		"Access",
	)
	s.AssertExportedFields(c, permissionDoc{}, fields)
}

func (s *MigrationSuite) TestModelUserLastConnectionDocFields(c *gc.C) {
	fields := set.NewStrings(
		// ID is the same as UserName (but lowercased)
		"ID",
		// ModelUUID shouldn't be exported, and is inherited
		// from the model definition.
		"ModelUUID",
		// UserName is captured in the migration.User.
		"UserName",
		"LastConnection",
	)
	s.AssertExportedFields(c, modelUserLastConnectionDoc{}, fields)
}

func (s *MigrationSuite) TestMachineDocFields(c *gc.C) {
	ignored := set.NewStrings(
		// DocID is the model + machine id
		"DocID",
		// ID is the machine id
		"Id",
		// ModelUUID shouldn't be exported, and is inherited
		// from the model definition.
		"ModelUUID",
		// Life is always alive, confirmed by export precheck.
		"Life",
		// ForceDestroyed is only true for dying/dead machines.
		"ForceDestroyed",
		// Ignored; they get populated on demand when the agent restarts
		"AgentStartedAt",
		"Hostname",
	)
	migrated := set.NewStrings(
		"Addresses",
		"Base",
		"ContainerType",
		"Jobs",
		"MachineAddresses",
		"Nonce",
		"PasswordHash",
		"Clean",
		"Volumes",
		"Filesystems",
		"Placement",
		"PreferredPrivateAddress",
		"PreferredPublicAddress",
		"Principals",
		"SupportedContainers",
		"SupportedContainersKnown",
		"Tools",
	)
	s.AssertExportedFields(c, machineDoc{}, migrated.Union(ignored))
}

func (s *MigrationSuite) TestInstanceDataFields(c *gc.C) {
	ignored := set.NewStrings(
		// KeepInstance is only set when a machine is
		// dying/dead (to be removed).
		"KeepInstance",
	)
	migrated := set.NewStrings(
		// DocID is the model + machine id
		"DocID",
		"MachineId",
		// ModelUUID shouldn't be exported, and is inherited
		// from the model definition.
		"ModelUUID",

		"InstanceId",
		"DisplayName",
		"Arch",
		"Mem",
		"RootDisk",
		"RootDiskSource",
		"CpuCores",
		"CpuPower",
		"Tags",
		"AvailZone",
		"VirtType",
		"CharmProfiles",
	)
	s.AssertExportedFields(c, instanceData{}, migrated.Union(ignored))
}

func (s *MigrationSuite) TestApplicationDocFields(c *gc.C) {
	ignored := set.NewStrings(
		// DocID is the model + name
		"DocID",
		// ModelUUID shouldn't be exported, and is inherited
		// from the model definition.
		"ModelUUID",
		// Always alive, not explicitly exported.
		"Life",
		// TxnRevno is mgo internals and should not be migrated.
		"TxnRevno",
		// UnitCount is handled by the number of units for the exported application.
		"UnitCount",
		// RelationCount is handled by the number of times the application name
		// appears in relation endpoints.
		"RelationCount",
	)
	migrated := set.NewStrings(
		"Name",
		"Subordinate",
		"CharmURL",
		"CharmModifiedVersion",
		"CharmOrigin",
		"ForceCharm",
		"Exposed",
		"ExposedEndpoints",
		"MinUnits",
		"MetricCredentials",
		"PasswordHash",
		"Tools",
		"DesiredScale",
		"Placement",
		"HasResources",
		"ProvisioningState",
	)
	s.AssertExportedFields(c, applicationDoc{}, migrated.Union(ignored))
}

func (s *MigrationSuite) TestUnitDocFields(c *gc.C) {
	ignored := set.NewStrings(
		"ModelUUID",
		"DocID",
		"Life",
		// Application is implicit in the migration structure through containment.
		"Application",
		// Resolved is not migrated as we check that all is good before we start.
		"Resolved",
		// Base and CharmURL also come from the application.
		"Base",
		"CharmURL",
	)
	migrated := set.NewStrings(
		"Name",
		"Principal",
		"Subordinates",
		"StorageAttachmentCount",
		"MachineId",
		"Tools",
		"PasswordHash",
	)
	s.AssertExportedFields(c, unitDoc{}, migrated.Union(ignored))
}

func (s *MigrationSuite) TestMachinePortRangesDocFields(c *gc.C) {
	fields := set.NewStrings(
		// DocID itself isn't migrated
		"DocID",
		// ModelUUID shouldn't be exported, and is inherited
		// from the model definition.
		"ModelUUID",
		// MachineID is implicit in the migration structure through containment.
		"MachineID",
		"UnitRanges",
		// TxnRevno isn't migrated.
		"TxnRevno",
	)
	s.AssertExportedFields(c, machinePortRangesDoc{}, fields)
}

func (s *MigrationSuite) TestMeterStatusDocFields(c *gc.C) {
	fields := set.NewStrings(
		// DocID itself isn't migrated
		"DocID",
		// ModelUUID shouldn't be exported, and is inherited
		// from the model definition.
		"ModelUUID",
		"Code",
		"Info",
	)
	s.AssertExportedFields(c, meterStatusDoc{}, fields)
}

func (s *MigrationSuite) TestRelationDocFields(c *gc.C) {
	fields := set.NewStrings(
		// DocID itself isn't migrated
		"DocID",
		// ModelUUID shouldn't be exported, and is inherited
		// from the model definition.
		"ModelUUID",
		"Key",
		"Id",
		"Endpoints",
		"Suspended",
		"SuspendedReason",
		// Life isn't exported, only alive.
		"Life",
		// UnitCount isn't explicitly exported, but defined by the stored
		// unit settings data for the relation endpoint.
		"UnitCount",
	)
	s.AssertExportedFields(c, relationDoc{}, fields)
	// We also need to check the Endpoint and nested charm.Relation field.
	endpointFields := set.NewStrings("ApplicationName", "Relation")
	s.AssertExportedFields(c, Endpoint{}, endpointFields)
	charmRelationFields := set.NewStrings(
		"Name",
		"Role",
		"Interface",
		"Optional",
		"Limit",
		"Scope",
	)
	s.AssertExportedFields(c, charm.Relation{}, charmRelationFields)
}

func (s *MigrationSuite) TestRelationScopeDocFields(c *gc.C) {
	fields := set.NewStrings(
		// DocID itself isn't migrated
		"DocID",
		// ModelUUID shouldn't be exported, and is inherited
		// from the model definition.
		"ModelUUID",
		"Key",
		// Departing isn't exported as we only deal with live, stable systems.
		"Departing",
	)
	s.AssertExportedFields(c, relationScopeDoc{}, fields)
}

func (s *MigrationSuite) TestAnnotatorDocFields(c *gc.C) {
	fields := set.NewStrings(
		// ModelUUID shouldn't be exported, and is inherited
		// from the model definition.
		"ModelUUID",
		"GlobalKey",
		"Tag",
		"Annotations",
	)
	s.AssertExportedFields(c, annotatorDoc{}, fields)
}

func (s *MigrationSuite) TestBlockDocFields(c *gc.C) {
	ignored := set.NewStrings(
		// The doc id is a sequence value that has no meaning.
		// It really doesn't need to be a sequence.
		"DocID",
		// ModelUUID shouldn't be exported, and is inherited
		// from the model definition.
		"ModelUUID",
		// Tag is just string representation of the model tag,
		// which also contains the model-uuid.
		"Tag",
	)
	migrated := set.NewStrings(
		"Type",
		"Message",
	)
	fields := migrated.Union(ignored)
	s.AssertExportedFields(c, blockDoc{}, fields)
}

func (s *MigrationSuite) TestSequenceDocFields(c *gc.C) {
	fields := set.NewStrings(
		// ModelUUID shouldn't be exported, and is inherited
		// from the model definition.
		"ModelUUID",
		"DocID",
		"Name",
		"Counter",
	)
	s.AssertExportedFields(c, sequenceDoc{}, fields)
}

func (s *MigrationSuite) TestConstraintsDocFields(c *gc.C) {
	fields := set.NewStrings(
		// ModelUUID shouldn't be exported, and is inherited
		// from the model definition.
		"ModelUUID",
		"DocID",
		"Arch",
		"CpuCores",
		"CpuPower",
		"Mem",
		"RootDisk",
		"RootDiskSource",
		"InstanceRole",
		"InstanceType",
		"Container",
		"Tags",
		"Spaces",
		"VirtType",
		"Zones",
		"AllocatePublicIP",
		"ImageID",
	)
	s.AssertExportedFields(c, constraintsDoc{}, fields)
}

func (s *MigrationSuite) TestHistoricalStatusDocFields(c *gc.C) {
	fields := set.NewStrings(
		// ModelUUID shouldn't be exported, and is inherited
		// from the model definition.
		"ModelUUID",
		"GlobalKey",
		"Status",
		"StatusInfo",
		"StatusData",
		"Updated",
	)
	s.AssertExportedFields(c, historicalStatusDoc{}, fields)
}

func (s *MigrationSuite) TestSpaceDocFields(c *gc.C) {
	ignored := set.NewStrings(
		"DocId",
		// Always alive, not explicitly exported.
		"Life",
	)
	migrated := set.NewStrings(
		"Id",
		"Name",
		"IsPublic",
		"ProviderId",
	)
	s.AssertExportedFields(c, spaceDoc{}, migrated.Union(ignored))
}

func (s *MigrationSuite) TestBlockDeviceFields(c *gc.C) {
	ignored := set.NewStrings(
		"DocID",
		"ModelUUID",
		// We manage machine through containment.
		"Machine",
	)
	migrated := set.NewStrings(
		"BlockDevices",
	)
	s.AssertExportedFields(c, blockDevicesDoc{}, migrated.Union(ignored))
	// The meat is in the type stored in "BlockDevices".
	migrated = set.NewStrings(
		"DeviceName",
		"DeviceLinks",
		"Label",
		"UUID",
		"HardwareId",
		"WWN",
		"BusAddress",
		"Size",
		"FilesystemType",
		"InUse",
		"MountPoint",
		"SerialId",
	)
	s.AssertExportedFields(c, BlockDeviceInfo{}, migrated)
}

func (s *MigrationSuite) TestSubnetDocFields(c *gc.C) {
	ignored := set.NewStrings(
		// DocID is the model + name
		"DocID",
		// TxnRevno is mgo internals and should not be migrated.
		"TxnRevno",
		// ModelUUID shouldn't be exported, and is inherited
		// from the model definition.
		"ModelUUID",
		// Always alive, not explicitly exported.
		"Life",
	)
	migrated := set.NewStrings(
		"CIDR",
		"ID",
		"VLANTag",
		"SpaceID",
		"ProviderId",
		"AvailabilityZones",
		"ProviderNetworkId",
		"FanLocalUnderlay",
		"FanOverlay",
		"IsPublic",
	)
	s.AssertExportedFields(c, subnetDoc{}, migrated.Union(ignored))
}

func (s *MigrationSuite) TestIPAddressDocFields(c *gc.C) {
	ignored := set.NewStrings(
		"DocID",
		"ModelUUID",
	)
	migrated := set.NewStrings(
		"DeviceName",
		"MachineID",
		"DNSSearchDomains",
		"GatewayAddress",
		"IsDefaultGateway",
		"ProviderID",
		"ProviderNetworkID",
		"ProviderSubnetID",
		"DNSServers",
		"SubnetCIDR",
		"ConfigMethod",
		"Value",
		"Origin",
		"IsShadow",
		"IsSecondary",
	)
	s.AssertExportedFields(c, ipAddressDoc{}, migrated.Union(ignored))
}

func (s *MigrationSuite) TestLinkLayerDeviceDocFields(c *gc.C) {
	ignored := set.NewStrings(
		"ModelUUID",
		"DocID",
	)
	migrated := set.NewStrings(
		"MachineID",
		"ProviderID",
		"Name",
		"MTU",
		"Type",
		"MACAddress",
		"IsAutoStart",
		"IsUp",
		"ParentName",
		"VirtualPortType",
	)
	s.AssertExportedFields(c, linkLayerDeviceDoc{}, migrated.Union(ignored))
}

func (s *MigrationSuite) TestSSHHostKeyDocFields(c *gc.C) {
	ignored := set.NewStrings()
	migrated := set.NewStrings(
		"Keys",
	)
	s.AssertExportedFields(c, sshHostKeysDoc{}, migrated.Union(ignored))
}

func (s *MigrationSuite) TestActionDocFields(c *gc.C) {
	ignored := set.NewStrings(
		"ModelUUID",
	)
	migrated := set.NewStrings(
		"DocId",
		"Receiver",
		"Name",
		"Operation",
		"Enqueued",
		"Started",
		"Completed",
		"Parameters",
		"Results",
		"Message",
		"Status",
		"Logs",
		"Parallel",
		"ExecutionGroup",
	)
	s.AssertExportedFields(c, actionDoc{}, migrated.Union(ignored))
}

func (s *MigrationSuite) TestOperationDocFields(c *gc.C) {
	ignored := set.NewStrings(
		"ModelUUID",
		"CompleteTaskCount",
	)
	migrated := set.NewStrings(
		"DocId",
		"Summary",
		"Enqueued",
		"Started",
		"Completed",
		"Status",
		"Fail",
		"SpawnedTaskCount",
	)
	s.AssertExportedFields(c, operationDoc{}, migrated.Union(ignored))
}

func (s *MigrationSuite) TestVolumeDocFields(c *gc.C) {
	ignored := set.NewStrings(
		"ModelUUID",
		"DocID",
		"Life",
		"HostId",    // recreated from pool properties
		"Releasing", // only when dying; can't migrate dying storage
	)
	migrated := set.NewStrings(
		"Name",
		"StorageId",
		"AttachmentCount", // through count of attachment instances
		"Info",
		"Params",
	)
	s.AssertExportedFields(c, volumeDoc{}, migrated.Union(ignored))
	// The info and params fields ar structs.
	s.AssertExportedFields(c, VolumeInfo{}, set.NewStrings(
		"HardwareId", "WWN", "Size", "Pool", "VolumeId", "Persistent"))
	s.AssertExportedFields(c, VolumeParams{}, set.NewStrings(
		"Size", "Pool"))
}

func (s *MigrationSuite) TestVolumeAttachmentDocFields(c *gc.C) {
	ignored := set.NewStrings(
		"ModelUUID",
		"DocID",
		"Life",
	)
	migrated := set.NewStrings(
		"Volume",
		"Host",
		"Info",
		"Params",
	)
	s.AssertExportedFields(c, volumeAttachmentDoc{}, migrated.Union(ignored))
	// The info and params fields ar structs.
	s.AssertExportedFields(c, VolumeAttachmentInfo{}, set.NewStrings(
		"DeviceName", "DeviceLink", "BusAddress", "ReadOnly", "PlanInfo"))
	s.AssertExportedFields(c, VolumeAttachmentParams{}, set.NewStrings(
		"ReadOnly"))
}

func (s *MigrationSuite) TestFilesystemDocFields(c *gc.C) {
	ignored := set.NewStrings(
		"ModelUUID",
		"DocID",
		"Life",
		"HostId",    // recreated from pool properties
		"Releasing", // only when dying; can't migrate dying storage
	)
	migrated := set.NewStrings(
		"FilesystemId",
		"StorageId",
		"VolumeId",
		"AttachmentCount", // through count of attachment instances
		"Info",
		"Params",
	)
	s.AssertExportedFields(c, filesystemDoc{}, migrated.Union(ignored))
	// The info and params fields ar structs.
	s.AssertExportedFields(c, FilesystemInfo{}, set.NewStrings(
		"Size", "Pool", "FilesystemId"))
	s.AssertExportedFields(c, FilesystemParams{}, set.NewStrings(
		"Size", "Pool"))
}

func (s *MigrationSuite) TestFilesystemAttachmentDocFields(c *gc.C) {
	ignored := set.NewStrings(
		"ModelUUID",
		"DocID",
		"Life",
	)
	migrated := set.NewStrings(
		"Filesystem",
		"Host",
		"Info",
		"Params",
	)
	s.AssertExportedFields(c, filesystemAttachmentDoc{}, migrated.Union(ignored))
	// The info and params fields ar structs.
	s.AssertExportedFields(c, FilesystemAttachmentInfo{}, set.NewStrings(
		"MountPoint", "ReadOnly"))
	s.AssertExportedFields(c, FilesystemAttachmentParams{}, set.NewStrings(
		"Location", "ReadOnly"))
}

func (s *MigrationSuite) TestStorageInstanceDocFields(c *gc.C) {
	ignored := set.NewStrings(
		"ModelUUID",
		"DocID",
		"Life",
		"Releasing", // only when dying; can't migrate dying storage
	)
	migrated := set.NewStrings(
		"Id",
		"Kind",
		"Owner",
		"StorageName",
		"AttachmentCount", // through count of attachment instances
		"Constraints",
	)
	s.AssertExportedFields(c, storageInstanceDoc{}, migrated.Union(ignored))
}

func (s *MigrationSuite) TestStorageAttachmentDocFields(c *gc.C) {
	ignored := set.NewStrings(
		"ModelUUID",
		"DocID",
		"Life",
	)
	migrated := set.NewStrings(
		"Unit",
		"StorageInstance",
	)
	s.AssertExportedFields(c, storageAttachmentDoc{}, migrated.Union(ignored))
}

func (s *MigrationSuite) TestStorageConstraintsDocFields(c *gc.C) {
	ignored := set.NewStrings(
		"ModelUUID",
		"DocID",
	)
	migrated := set.NewStrings(
		"Constraints",
	)
	s.AssertExportedFields(c, storageConstraintsDoc{}, migrated.Union(ignored))
}

func (s *MigrationSuite) TestPayloadDocFields(c *gc.C) {
	definedThroughContainment := set.NewStrings(
		"UnitID",
		"MachineID",
	)
	migrated := set.NewStrings(
		"Name",
		"Type",
		"RawID",
		"State",
		"Labels",
	)
	s.AssertExportedFields(c, payloadDoc{}, migrated.Union(definedThroughContainment))
}

func (s *MigrationSuite) TestEndpointBindingFields(c *gc.C) {
	definedThroughContainment := set.NewStrings(
		"DocID",
	)
	migrated := set.NewStrings(
		"Bindings",
	)
	ignored := set.NewStrings(
		"TxnRevno",
	)
	fields := definedThroughContainment.Union(migrated).Union(ignored)
	s.AssertExportedFields(c, endpointBindingsDoc{}, fields)
}

func (s *MigrationSuite) AssertExportedFields(c *gc.C, doc interface{}, fields set.Strings) {
	expected := testing.GetExportedFields(doc)
	unknown := expected.Difference(fields)
	removed := fields.Difference(expected)
	// If this test fails, it means that extra fields have been added to the
	// doc without thinking about the migration implications.
	c.Check(unknown, gc.HasLen, 0)
	c.Assert(removed, gc.HasLen, 0)
}

func (s *MigrationSuite) TestSecretMetadataDocFields(c *gc.C) {
	ignored := set.NewStrings(
		"DocID",

		// These are not exported but instead
		// calculated from the revisions.
		"LatestRevision",
		"LatestExpireTime",
	)
	migrated := set.NewStrings(
		"Version",
		"OwnerTag",
		"Description",
		"Label",
		"RotatePolicy",
		"LatestRevisionChecksum",
		"AutoPrune",
		"CreateTime",
		"UpdateTime",
	)
	s.AssertExportedFields(c, secretMetadataDoc{}, migrated.Union(ignored))
}

func (s *MigrationSuite) TestSecretRevisionDocFields(c *gc.C) {
	ignored := set.NewStrings(
		"DocID",
		"TxnRevno",
	)
	migrated := set.NewStrings(
		"Revision",
		"CreateTime",
		"UpdateTime",
		"ExpireTime",
		"Obsolete",
		"ValueRef",
		"Data",
		"OwnerTag",
		"PendingDelete",
	)
	s.AssertExportedFields(c, secretRevisionDoc{}, migrated.Union(ignored))
}

func (s *MigrationSuite) TestSecretRotationDocFields(c *gc.C) {
	ignored := set.NewStrings(
		"DocID",
		"TxnRevno",
	)
	migrated := set.NewStrings(
		"NextRotateTime",
		"OwnerTag",
	)
	s.AssertExportedFields(c, secretRotationDoc{}, migrated.Union(ignored))
}

func (s *MigrationSuite) TestSecretConsumerDocFields(c *gc.C) {
	ignored := set.NewStrings(
		"DocID",
	)
	migrated := set.NewStrings(
		"ConsumerTag",
		"Label",
		"CurrentRevision",
		"LatestRevision",
	)
	s.AssertExportedFields(c, secretConsumerDoc{}, migrated.Union(ignored))
}
