// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"reflect"

	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"
)

type MigrationSuite struct{}

var _ = gc.Suite(&MigrationSuite{})

func (s *MigrationSuite) TestKnownCollections(c *gc.C) {
	completedCollections := set.NewStrings(
		environmentsC,
		envUsersC,
		envUserLastConnectionC,
		settingsC,
		statusesC,

		// machine
		cloudimagemetadataC,
		instanceDataC,
		machinesC,

		// service
		servicesC,
		// settings reference counts are only used for services
		settingsrefsC,
	)

	ignoredCollections := set.NewStrings(
		// We don't export the controller environment at this stage.
		stateServersC,
		// Users aren't migrated.
		usersC,
		userLastLoginC,
		// userenvnameC is just to provide a unique key constraint.
		userenvnameC,
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
		// Transaction stuff.
		"txns",
		"txns.log",

		// The container ref document is primarily there to keep track
		// of a particular machine's containers. The migration format
		// uses object containment for this purpose.
		containerRefsC,
		// The min units collection is only used to trigger a watcher
		// in order to have the service add or remove units if the minimum
		// number of units is changed. The Service doc has all we need
		// for migratino.
		minUnitsC,
	)

	// THIS SET WILL BE REMOVED WHEN MIGRATIONS ARE COMPLETE
	todoCollections := set.NewStrings(
		// environment
		blocksC,
		cleanupsC,
		sequenceC,

		// machine
		rebootC,

		// service / unit
		assignUnitC,
		charmsC,
		leasesC,
		openedPortsC,
		"payloads",
		unitsC,

		// relation
		relationsC,
		relationScopesC,

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
		ipaddressesC,
		networksC,
		networkInterfacesC,
		requestedNetworksC,
		subnetsC,
		spacesC,

		// actions
		actionsC,
		actionNotificationsC,
		actionresultsC,

		// done as part of machines/services/units
		annotationsC,
		constraintsC,
		statusesHistoryC,

		// uncategorised
		meterStatusC,    // red / green status for metrics / charms
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

func (s *MigrationSuite) TestEnvironmentDocFields(c *gc.C) {
	fields := set.NewStrings(
		// UUID and Mame are constructed from the model config.
		"UUID",
		"Name",
		// Life will always be alive, or we won't be migrating.
		"Life",
		"Owner",
		"LatestAvailableTools",
		// ServerUUID is recreated when the new model is created in the
		// new controller (yay name changes).
		"ServerUUID",
		// Both of the times for dying and death are empty as the model
		// is alive.
		"TimeOfDying",
		"TimeOfDeath",
	)
	s.AssertExportedFields(c, environmentDoc{}, fields)
}

func (s *MigrationSuite) TestEnvUserDocFields(c *gc.C) {
	fields := set.NewStrings(
		// ID is the same as UserName (but lowercased)
		"ID",
		// EnvUUID shouldn't be exported, and is inherited
		// from the model definition.
		"EnvUUID",
		// Tracked fields:
		"UserName",
		"DisplayName",
		"CreatedBy",
		"DateCreated",
		"ReadOnly",
	)
	s.AssertExportedFields(c, envUserDoc{}, fields)
}

func (s *MigrationSuite) TestEnvUserLastConnectionDocFields(c *gc.C) {
	fields := set.NewStrings(
		// ID is the same as UserName (but lowercased)
		"ID",
		// EnvUUID shouldn't be exported, and is inherited
		// from the model definition.
		"EnvUUID",
		// UserName is captured in the migration.User.
		"UserName",
		"LastConnection",
	)
	s.AssertExportedFields(c, envUserLastConnectionDoc{}, fields)
}

func (s *MigrationSuite) TestMachineDocFields(c *gc.C) {
	fields := set.NewStrings(
		// DocID is the env + machine id
		"DocID",
		// ID is the machine id
		"Id",
		// EnvUUID shouldn't be exported, and is inherited
		// from the model definition.
		"EnvUUID",
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
		"Series",
		"SupportedContainers",
		"SupportedContainersKnown",
		"Tools",
	)
	todo := set.NewStrings(
		"Principals",
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
		// EnvUUID shouldn't be exported, and is inherited
		// from the model definition.
		"EnvUUID",

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
	fields := set.NewStrings(
		// DocID is the env + name
		"DocID",
		// EnvUUID shouldn't be exported, and is inherited
		// from the model definition.
		"EnvUUID",
		// Always alive, not explicitly exported.
		"Life",
		// OwnerTag is deprecated and should be deleted.
		"OwnerTag",
		// TxnRevno is mgo internals and no idea why this is read.
		"TxnRevno",

		"Name",
		"Series",
		"Subordinate",
		"CharmURL",
		"ForceCharm",
		"Exposed",
		"MinUnits",
	)
	todo := set.NewStrings(
		"UnitCount",
		"RelationCount",
		"MetricCredentials",
	)
	s.AssertExportedFields(c, serviceDoc{}, fields.Union(todo))
}

func (s *MigrationSuite) AssertExportedFields(c *gc.C, doc interface{}, fields set.Strings) {
	expected := getExportedFields(doc)
	unknown := expected.Difference(fields)
	// If this test fails, it means that extra fields have been added to the
	// doc without thinking about the migration implications.
	c.Assert(unknown, gc.HasLen, 0)
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
