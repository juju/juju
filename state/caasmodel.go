// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// CAASModel contains functionality that is specific to an
// Containers-As-A-Service (CAAS) model. It embeds a Model so that
// all generic Model functionality is also available.
type CAASModel struct {
	// TODO(caas) - this is all still messy until things shake out.
	*Model
}

// CAASModel returns an Containers-As-A-Service (CAAS) model.
func (m *Model) CAASModel() (*CAASModel, error) {
	return &CAASModel{
		Model: m,
	}, nil
}
