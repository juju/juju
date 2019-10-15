// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor27 returns upgrade steps for Juju 2.7.0.
func stateStepsFor27() []Step {
	return []Step{
		&upgradeStep{
			description: "add controller node docs",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().AddControllerNodeDocs()
			},
		},
		&upgradeStep{
			description: "recreate spaces with IDs",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().AddSpaceIdToSpaceDocs()
			},
		},
		&upgradeStep{
			description: "change subnet AvailabilityZone to AvailabilityZones",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().ChangeSubnetAZtoSlice()
			},
		},
		&upgradeStep{
			description: "change subnet SpaceName to SpaceID",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().ChangeSubnetSpaceNameToSpaceID()
			},
		},
		&upgradeStep{
			description: "recreate subnets with IDs",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().AddSubnetIdToSubnetDocs()
			},
		},
		&upgradeStep{
			description: "replace portsDoc.SubnetID as a CIDR with an ID.",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().ReplacePortsDocSubnetIDCIDR()
			},
		},
		&upgradeStep{
			description: "ensure application settings exist for all relations",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().EnsureRelationApplicationSettings()
			},
		},
		&upgradeStep{
			description: "ensure stored addresses refer to space by ID, and remove old space name/provider ID",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().ConvertAddressSpaceIDs()
			},
		},
		&upgradeStep{
			description: "replace space name in endpointBindingDoc bindings with an space ID",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().ReplaceSpaceNameWithIDEndpointBindings()
			},
		},
	}
}
