// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
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
		envUsersC,
		envUserLastConnectionC,
		blocksC,
		cleanupsC,
		sequenceC,
		leasesC,
		charmsC,
		servicesC,
		unitsC,
		minUnitsC,
		assignUnitC,
		meterStatusC,
		settingsrefsC,
		relationsC,
		relationScopesC,
		containerRefsC,
		instanceDataC,
		machinesC,
		rebootC,
		blockDevicesC,
		filesystemsC,
		filesystemAttachmentsC,
		storageInstancesC,
		storageAttachmentsC,
		volumesC,
		volumeAttachmentsC,
		ipaddressesC,
		networkInterfacesC,
		networksC,
		openedPortsC,
		requestedNetworksC,
		subnetsC,
		actionsC,
		actionNotificationsC,
		"payloads",
		annotationsC,
		settingsC,
		constraintsC,
		storageConstraintsC,
		statusesC,
		statusesHistoryC,
		spacesC,
		cloudimagemetadataC,
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
