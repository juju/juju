// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import "github.com/juju/errors"

// IAASModel contains functionality that is specific to an
// Infrastructure-As-A-Service (IAAS) model. It embeds a Model so that
// all generic Model functionality is also available.
type IAASModel struct {
	*Model

	mb modelBackend

	// TODO(caas): This should be removed once things
	// have been sufficiently untangled.
	st *State
}

// IAASModel returns an Infrastructure-As-A-Service (IAAS) model.
func (m *Model) IAASModel() (*IAASModel, error) {
	// TODO: error when model type is not IAAS.
	return &IAASModel{
		Model: m,
		mb:    m.st,
		st:    m.st,
	}, nil
}

// IAASModel returns an Infrastructure-As-A-Service (IAAS) model.
//
// TODO(caas): This is a convenience helper only and will go away
// once most model related functionality has been moved from State to
// Model/IAASModel. Model.IAASModel() should be preferred where-ever
// possible.
func (st *State) IAASModel() (*IAASModel, error) {
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	im, err := m.IAASModel()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return im, nil
}
