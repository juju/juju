// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

var (
	NewApplication = newApplication
)

// Expose SetDetails for testing.

func (a *Application) SetDetails(details ApplicationChange) {
	a.setDetails(details)
}

func (m *Model) SetDetails(details ModelChange) {
	m.setDetails(details)
}

// Expose Remove* for testing

func (m *Model) RemoveCharm(ch RemoveCharm) error {
	return m.removeCharm(ch)
}

func (m *Model) RemoveUnit(ch RemoveUnit) error {
	return m.removeUnit(ch)
}

func (m *Model) RemoveMachine(details RemoveMachine) error {
	return m.removeMachine(details)
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
