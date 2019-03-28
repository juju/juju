// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

var (
	CreateControllerGauges = createControllerGauges
	NewModel               = newModel
	NewApplication         = newApplication
)

// Expose SetDetails for testing.

func (m *Model) SetDetails(details ModelChange) {
	m.setDetails(details)
}

func (a *Application) SetDetails(details ApplicationChange) {
	a.setDetails(details)
}

// Expose Update* for testing.

func (m *Model) UpdateMachine(details MachineChange) {
	m.updateMachine(details)
}

func (m *Model) UpdateUnit(details UnitChange) {
	m.updateUnit(details)
}
