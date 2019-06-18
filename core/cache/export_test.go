// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

// Expose SetDetails for testing.

func (a *Application) SetDetails(details ApplicationChange) {
	a.setDetails(details)
}

func (m *Model) SetDetails(details ModelChange) {
	m.setDetails(details)
}

// Expose Remove* for testing

func (m *Model) RemoveCharm(details RemoveCharm) error {
	return m.removeCharm(details)
}

func (m *Model) RemoveUnit(details RemoveUnit) error {
	return m.removeUnit(details)
}

func (m *Model) RemoveMachine(details RemoveMachine) error {
	return m.removeMachine(details)
}

func (m *Model) RemoveBranch(details RemoveBranch) error {
	return m.removeBranch(details)
}

// Expose Update* for testing.

func (m *Model) UpdateMachine(details MachineChange, manager *residentManager) {
	m.updateMachine(details, manager)
}

func (m *Model) UpdateUnit(details UnitChange, manager *residentManager) {
	m.updateUnit(details, manager)
}

func (m *Model) UpdateApplication(details ApplicationChange, manager *residentManager) {
	m.updateApplication(details, manager)
}

func (m *Model) UpdateCharm(details CharmChange, manager *residentManager) {
	m.updateCharm(details, manager)
}

func (m *Model) UpdateBranch(details BranchChange, manager *residentManager) {
	m.updateBranch(details, manager)
}
