// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"reflect"

	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
)

type MigrationSuite struct{}

var _ = gc.Suite(&MigrationSuite{})

func (s *MigrationSuite) TestKnownCollections(c *gc.C) {
	completedCollections := set.NewStrings(
		annotationsC,
		blocksC,
		constraintsC,
		modelsC,
		modelUsersC,
		modelUserLastConnectionC,
		permissionsC,
		settingsC,
		sequenceC,
		sshHostKeysC,
		statusesC,
		statusesHistoryC,

		// machine
		instanceDataC,
		machinesC,
		openedPortsC,

		// application / unit
		leasesC,
		applicationsC,
		unitsC,
		meterStatusC, // red / green status for metrics of units
		payloadsC,

		// relation
		relationsC,
		relationScopesC,

		// networking
		ipAddressesC,
		spacesC,
		linkLayerDevicesC,
		subnetsC,

		// storage
		blockDevicesC,

		// actions
		actionsC,

		// storage
		filesystemsC,
		filesystemAttachmentsC,
		storageAttachmentsC,
		storageConstraintsC,
		storageInstancesC,
		volumesC,
		volumeAttachmentsC,
	)

	ignoredCollections := set.NewStrings(
		// Precheck ensures that there are no cleanup docs or pending
		// machine removals.
		cleanupsC,
		machineRemovalsC,
		// We don't export the controller model at this stage.
		controllersC,
		// Clouds aren't migrated. They must exist in the
		// target controller already.
		cloudsC,
		// Cloud credentials aren't migrated. They must exist in the
		// target controller already.
		cloudCredentialsC,
		// This is controller global, and related to the system state of the
		// embedded GUI.
		guimetadataC,
		// This is controller global, not migrated.
		guisettingsC,
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
		// Backup and restore information is not migrated.
		restoreInfoC,
		// reference counts are implementation details that should be
		// reconstructed on the other side.
		refcountsC,
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

		// We don't import any of the migration collections.
		migrationsC,
		migrationsStatusC,
		migrationsActiveC,
		migrationsMinionSyncC,

		// The container ref document is primarily there to keep track
		// of a particular machine's containers. The migration format
		// uses object containment for this purpose.
		containerRefsC,
		// The min units collection is only used to trigger a watcher
		// in order to have the service add or remove units if the minimum
		// number of units is changed. The Service doc has all we need
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
		linkLayerDevicesRefsC,

		// Recreated whilst migrating actions.
		actionNotificationsC,
	)

	// THIS SET WILL BE REMOVED WHEN MIGRATIONS ARE COMPLETE
	todoCollections := set.NewStrings(
		// model configuration
		globalSettingsC,

		// model
		cloudimagemetadataC,

		// machine
		rebootC,

		// service / unit
		charmsC,
		"resources",
		endpointBindingsC,

		// uncategorised
		metricsManagerC, // should really be copied across
		auditingC,
	)

	envCollections := set.NewStrings()
	for name := range allCollections() {
		envCollections.Add(name)
	}

	known := completedCollections.Union(ignoredCollections)

	remainder := envCollections.Difference(known)
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
		// ControllerUUID is recreated when the new model
		// is created in the new controller (yay name changes).
		"ControllerUUID",

		"MigrationMode",
		"Owner",
		"Cloud",
		"CloudRegion",
		"CloudCredential",
		"LatestAvailableTools",
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

func (s *MigrationSuite) TestEnvUserLastConnectionDocFields(c *gc.C) {
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
		// DocID is the env + machine id
		"DocID",
		// ID is the machine id
		"Id",
		// ModelUUID shouldn't be exported, and is inherited
		// from the model definition.
		"ModelUUID",
		// Life is always alive, confirmed by export precheck.
		"Life",
		// NoVote and HasVote only matter for machines with manage state job
		// and we don't support migrating the controller model.
		"NoVote",
		"HasVote",
		// Ignored at this stage, could be an issue if mongo 3.0 isn't
		// available.
		"StopMongoUntilVersion",
	)
	migrated := set.NewStrings(
		"Addresses",
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
		"Series",
		"SupportedContainers",
		"SupportedContainersKnown",
		"Tools",
	)
	s.AssertExportedFields(c, machineDoc{}, migrated.Union(ignored))
}

func (s *MigrationSuite) TestInstanceDataFields(c *gc.C) {
	fields := set.NewStrings(
		// DocID is the env + machine id
		"DocID",
		"MachineId",
		// ModelUUID shouldn't be exported, and is inherited
		// from the model definition.
		"ModelUUID",

		"InstanceId",
		"Status",
		"Arch",
		"Mem",
		"RootDisk",
		"CpuCores",
		"CpuPower",
		"Tags",
		"AvailZone",
	)
	s.AssertExportedFields(c, instanceData{}, fields)
}

func (s *MigrationSuite) TestApplicationDocFields(c *gc.C) {
	ignored := set.NewStrings(
		// DocID is the env + name
		"DocID",
		// ModelUUID shouldn't be exported, and is inherited
		// from the model definition.
		"ModelUUID",
		// Always alive, not explicitly exported.
		"Life",
		// TxnRevno is mgo internals and should not be migrated.
		"TxnRevno",
		// UnitCount is handled by the number of units for the exported service.
		"UnitCount",
		// RelationCount is handled by the number of times the application name
		// appears in relation endpoints.
		"RelationCount",
	)
	migrated := set.NewStrings(
		"Name",
		"Series",
		"Subordinate",
		"CharmURL",
		"Channel",
		"CharmModifiedVersion",
		"ForceCharm",
		"Exposed",
		"MinUnits",
		"MetricCredentials",
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
		// Series and CharmURL also come from the service.
		"Series",
		"CharmURL",
		"TxnRevno",
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

func (s *MigrationSuite) TestPortsDocFields(c *gc.C) {
	fields := set.NewStrings(
		// DocID itself isn't migrated
		"DocID",
		// ModelUUID shouldn't be exported, and is inherited
		// from the model definition.
		"ModelUUID",
		// MachineID is implicit in the migration structure through containment.
		"MachineID",
		"SubnetID",
		"Ports",
		// TxnRevno isn't migrated.
		"TxnRevno",
	)
	s.AssertExportedFields(c, portsDoc{}, fields)
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

func (s *MigrationSuite) TestAnnatatorDocFields(c *gc.C) {
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
		"Arch",
		"CpuCores",
		"CpuPower",
		"Mem",
		"RootDisk",
		"InstanceType",
		"Container",
		"Tags",
		"Spaces",
		"VirtType",
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
		// Always alive, not explicitly exported.
		"Life",
	)
	migrated := set.NewStrings(
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
		"BusAddress",
		"Size",
		"FilesystemType",
		"InUse",
		"MountPoint",
	)
	s.AssertExportedFields(c, BlockDeviceInfo{}, migrated)
}

func (s *MigrationSuite) TestSubnetDocFields(c *gc.C) {
	ignored := set.NewStrings(
		// DocID is the env + name
		"DocID",
		// ModelUUID shouldn't be exported, and is inherited
		// from the model definition.
		"ModelUUID",
		// Always alive, not explicitly exported.
		"Life",

		// Currently unused (never set or exposed).
		"IsPublic",
	)
	migrated := set.NewStrings(
		"CIDR",
		"VLANTag",
		"SpaceName",
		"ProviderId",
		"AvailabilityZone",
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
		"ProviderID",
		"DNSServers",
		"SubnetCIDR",
		"ConfigMethod",
		"Value",
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
		"Enqueued",
		"Started",
		"Completed",
		"Parameters",
		"Results",
		"Message",
		"Status",
	)
	s.AssertExportedFields(c, actionDoc{}, migrated.Union(ignored))
}

func (s *MigrationSuite) TestVolumeDocFields(c *gc.C) {
	ignored := set.NewStrings(
		"ModelUUID",
		"DocID",
		"Life",
	)
	migrated := set.NewStrings(
		"Name",
		"StorageId",
		"AttachmentCount", // through count of attachment instances
		"Binding",
		"Info",
		"Params",
	)
	s.AssertExportedFields(c, volumeDoc{}, migrated.Union(ignored))
	// The info and params fields ar structs.
	s.AssertExportedFields(c, VolumeInfo{}, set.NewStrings(
		"HardwareId", "Size", "Pool", "VolumeId", "Persistent"))
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
		"Machine",
		"Info",
		"Params",
	)
	s.AssertExportedFields(c, volumeAttachmentDoc{}, migrated.Union(ignored))
	// The info and params fields ar structs.
	s.AssertExportedFields(c, VolumeAttachmentInfo{}, set.NewStrings(
		"DeviceName", "DeviceLink", "BusAddress", "ReadOnly"))
	s.AssertExportedFields(c, VolumeAttachmentParams{}, set.NewStrings(
		"ReadOnly"))
}

func (s *MigrationSuite) TestFilesystemDocFields(c *gc.C) {
	ignored := set.NewStrings(
		"ModelUUID",
		"DocID",
		"Life",
	)
	migrated := set.NewStrings(
		"FilesystemId",
		"StorageId",
		"VolumeId",
		"AttachmentCount", // through count of attachment instances
		"Binding",
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
		"Machine",
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
	)
	migrated := set.NewStrings(
		"Id",
		"Kind",
		"Owner",
		"StorageName",
		"AttachmentCount", // through count of attachment instances
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

func (s *MigrationSuite) AssertExportedFields(c *gc.C, doc interface{}, fields set.Strings) {
	expected := getExportedFields(doc)
	unknown := expected.Difference(fields)
	removed := fields.Difference(expected)
	// If this test fails, it means that extra fields have been added to the
	// doc without thinking about the migration implications.
	c.Check(unknown, gc.HasLen, 0)
	c.Assert(removed, gc.HasLen, 0)
}

func getExportedFields(arg interface{}) set.Strings {
	t := reflect.TypeOf(arg)
	result := set.NewStrings()

	count := t.NumField()
	for i := 0; i < count; i++ {
		f := t.Field(i)
		// empty PkgPath means exported field.
		// see https://golang.org/pkg/reflect/#StructField
		if f.PkgPath == "" {
			result.Add(f.Name)
		}
	}

	return result
}
