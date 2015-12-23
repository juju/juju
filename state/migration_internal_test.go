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
	)

	// THIS SET WILL BE REMOVED WHEN MIGRATIONS ARE COMPLETE
	todoCollections := set.NewStrings(
		// environment
		blocksC,
		cleanupsC,
		cloudimagemetadataC,
		envUsersC,
		envUserLastConnectionC,
		sequenceC,

		// machine
		containerRefsC,
		instanceDataC,
		machinesC,
		rebootC,
		ipaddressesC,

		// service / unit
		assignUnitC,
		charmsC,
		leasesC,
		minUnitsC,
		openedPortsC,
		"payloads",
		servicesC,
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
		networksC,
		networkInterfacesC,
		requestedNetworksC,
		subnetsC,
		spacesC,

		// actions
		actionsC,
		actionNotificationsC,

		// done as part of machines/services/units
		annotationsC,
		constraintsC,
		settingsC,
		statusesC,
		statusesHistoryC,

		// uncategorised
		meterStatusC,
		settingsrefsC,
	)

	envCollections := set.NewStrings()
	for name, info := range allCollections() {
		if !info.global {
			envCollections.Add(name)
		}
	}

	remainder := envCollections.Difference(completedCollections)
	remainder = remainder.Difference(todoCollections)

	// If this test fails, it means that a new collection has been added
	// but migrations for it has not been done. This is a Bad Thing™.
	c.Assert(remainder, gc.HasLen, 0)
}

func (s *MigrationSuite) TestEnvironmentDocFields(c *gc.C) {
	fields := set.NewStrings(
		// uuid and name are constructed from the model config
		"UUID",
		"Name",
		// life will always be alive, or we won't be migrating
		"Life",
		"Owner",
		// server uuid is recreated when the new model is created in the
		// new controller (yay name changes).
		"ServerUUID",
		// Both of the times for dying and death are empty as the model
		// is alive.
		"TimeOfDying",
		"TimeOfDeath",
	)
	expected := getExportedFields(environmentDoc{})
	unknown := expected.Difference(fields)
	// If this test fails, it means that extra fields have been added to the
	// environmentDoc without thinking about the migration implications.
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
