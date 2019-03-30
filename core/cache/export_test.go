// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

var (
	CreateControllerGauges = createControllerGauges
	NewApplication         = newApplication
	NewModel               = newModel
)

// Expose SetDetails for testing.

func (a *Application) SetDetails(details ApplicationChange) {
	a.setDetails(details)
}

func (m *Model) SetDetails(details ModelChange) {
	m.setDetails(details)
}

// Expose Remove* for testing

func (m *Model) RemoveCharm(ch RemoveCharm) {
	m.removeCharm(ch)
}

func (m *Model) RemoveUnit(ch RemoveUnit) {
	m.removeUnit(ch)
}

// Expose Update* for testing.

func (m *Model) UpdateMachine(details MachineChange) {
	m.updateMachine(details)
}

func (m *Model) UpdateUnit(details UnitChange) {
	m.updateUnit(details)
}

func (m *Model) UpdateApplication(details ApplicationChange) {
	m.updateApplication(details)
}

func (m *Model) UpdateCharm(details CharmChange) {
	m.updateCharm(details)
}
