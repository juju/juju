// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor3629 returns upgrade steps for Juju 3.6.29 that manipulate
// state directly.
func stateStepsFor3629() []Step {
	return []Step{
		// Repair counts first since subsequent steps depend on correct counts.
		&upgradeStep{
			description: "repair application relation counts",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().FixApplicationCounts()
			},
		},
		&upgradeStep{
			description: "remove relations with orphaned application references",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().RemoveOrphanedApplicationRelations()
			},
		},
		&upgradeStep{
			description: "remove orphaned relation docs",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().RemoveOrphanedRelationDocs()
			},
		},
		&upgradeStep{
			description: "remove orphaned unit state relations",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().RemoveOrphanedUnitStateRelations()
			},
		},
	}
}
