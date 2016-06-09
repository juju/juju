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
		settingsC,
		sequenceC,
		statusesC,
		statusesHistoryC,

		// machine
		instanceDataC,
		machinesC,
		openedPortsC,

		// service / unit
		leasesC,
		applicationsC,
		unitsC,
		meterStatusC, // red / green status for metrics of units

		// settings reference counts are only used for applications
		settingsrefsC,

		// relation
		relationsC,
		relationScopesC,
	)

	ignoredCollections := set.NewStrings(
		// Precheck ensures that there are no cleanup docs.
		cleanupsC,
		// We don't export the controller model at this stage.
		controllersC,
		// This is controller global, and related to the system state of the
		// embedded GUI.
		guimetadataC,
		// This is controller global, not migrated.
		guisettingsC,
		// Users aren't migrated.
		usersC,
		userLastLoginC,
		// userenvnameC is just to provide a unique key constraint.
		usermodelnameC,
		// Metrics aren't migrated.
		metricsC,
		// leaseC is deprecated in favour of leasesC.
		leaseC,
		// Backup and restore information is not migrated.
		restoreInfoC,
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

		// This has been deprecated in 2.0, and should not contain any data
		// we actually care about migrating.
		legacyipaddressesC,

		// The SSH host keys for each machine will be reported as each
		// machine agent starts up.
		sshHostKeysC,
	)

	// THIS SET WILL BE REMOVED WHEN MIGRATIONS ARE COMPLETE
	todoCollections := set.NewStrings(
		// model
		cloudimagemetadataC,

		// machine
		rebootC,

		// service / unit
		charmsC,
		"payloads",
		"resources",
		endpointBindingsC,

		// storage
		blockDevicesC,
		filesystemsC,
		filesystemAttachmentsC,
		storageInstancesC,
		storageAttachmentsC,
		storageConstraintsC,
		volumesC,
		volumeAttachmentsC,

		// network
		ipAddressesC,
		providerIDsC,
		linkLayerDevicesC,
		linkLayerDevicesRefsC,
		subnetsC,
		spacesC,

		// actions
		actionsC,
		actionNotificationsC,
		actionresultsC,

		// uncategorised
		metricsManagerC, // should really be copied across
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
	c.Assert(remainder, gc.HasLen, 0)
}

func (s *MigrationSuite) TestModelDocFields(c *gc.C) {
	fields := set.NewStrings(
		// UUID and Name are constructed from the model config.
		"UUID",
		"Name",
		// Life will always be alive, or we won't be migrating.
		"Life",
		// ServerUUID is recreated when the new model is created in the
		// new controller (yay name changes).
		"ServerUUID",

		"MigrationMode",
		"Owner",
		"Cloud",
		"LatestAvailableTools",
	)
	s.AssertExportedFields(c, modelDoc{}, fields)
}

func (s *MigrationSuite) TestEnvUserDocFields(c *gc.C) {
	fields := set.NewStrings(
		// ID is the same as UserName (but lowercased)
		"ID",
		// ModelUUID shouldn't be exported, and is inherited
		// from the model definition.
		"ModelUUID",
		// Tracked fields:
		"UserName",
		"DisplayName",
		"CreatedBy",
		"DateCreated",
		"Access",
	)
	s.AssertExportedFields(c, modelUserDoc{}, fields)
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
	fields := set.NewStrings(
		// DocID is the env + machine id
		"DocID",
		// ID is the machine id
		"Id",
		// ModelUUID shouldn't be exported, and is inherited
		// from the model definition.
		"ModelUUID",
		// Life is always alive, confirmed by export precheck.
		"Life",

		"Addresses",
		"ContainerType",
		"Jobs",
		"MachineAddresses",
		"Nonce",
		"PasswordHash",
		"Placement",
		"PreferredPrivateAddress",
		"PreferredPublicAddress",
		"Principals",
		"Series",
		"SupportedContainers",
		"SupportedContainersKnown",
		"Tools",

		// Ignored at this stage, could be an issue if mongo 3.0 isn't
		// available.
		"StopMongoUntilVersion",
	)
	todo := set.NewStrings(
		"Volumes",
		"NoVote",
		"Clean",
		"Filesystems",
		"HasVote",
	)
	s.AssertExportedFields(c, machineDoc{}, fields.Union(todo))
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

func (s *MigrationSuite) TestServiceDocFields(c *gc.C) {
	ignored := set.NewStrings(
		// DocID is the env + name
		"DocID",
		// ModelUUID shouldn't be exported, and is inherited
		// from the model definition.
		"ModelUUID",
		// Always alive, not explicitly exported.
		"Life",
		// OwnerTag is deprecated and should be deleted.
		"OwnerTag",
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

func (s *MigrationSuite) TestSettingsRefsDocFields(c *gc.C) {
	fields := set.NewStrings(
		// ModelUUID shouldn't be exported, and is inherited
		// from the model definition.
		"ModelUUID",

		"RefCount",
	)
	s.AssertExportedFields(c, settingsRefsDoc{}, fields)
}

func (s *MigrationSuite) TestUnitDocFields(c *gc.C) {
	fields := set.NewStrings(
		// DocID itself isn't migrated
		"DocID",
		"Name",
		// ModelUUID shouldn't be exported, and is inherited
		// from the model definition.
		"ModelUUID",
		// Application is implicit in the migration structure through containment.
		"Application",
		// Series, CharmURL, and Channel also come from the service.
		"Series",
		"CharmURL",
		"Principal",
		"Subordinates",
		"MachineId",
		// Resolved is not migrated as we check that all is good before we start.
		"Resolved",
		"Tools",
		// Life isn't migrated as we only migrate live things.
		"Life",
		// TxnRevno isn't migrated.
		"TxnRevno",
		"PasswordHash",
		// Obsolete and not migrated.
		"Ports",
		"PublicAddress",
		"PrivateAddress",
	)
	todo := set.NewStrings(
		"StorageAttachmentCount",
	)

	s.AssertExportedFields(c, unitDoc{}, fields.Union(todo))
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
