// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

var (
	CreateControllerGauges = createControllerGauges
	NewModel               = newModel
)

// Expose SetDetails for testing.
func (m *Model) SetDetails(details ModelChange) {
	m.setDetails(details)
}
