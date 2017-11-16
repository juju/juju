// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import "github.com/juju/errors"

// IAASModel contains functionality that is specific to an
// Infrastructure-As-A-Service (IAAS) model. It embeds a Model so that
// all generic Model functionality is also available.
type IAASModel struct {
	// TODO(caas) - this is all still messy until things shake out.
	*Model
	mb modelBackend
}

// IAASModel returns an Infrastructure-As-A-Service (IAAS) model.
func (m *Model) IAASModel() (*IAASModel, error) {
	if m.Type() != ModelTypeIAAS {
		return nil, errors.NotSupportedf("called IAASModel() on a non-IAAS Model")
	}
	return &IAASModel{
		Model: m,
		mb:    m.st,
	}, nil
}

// CloudRegion returns the name of the cloud region to which the model is deployed.
func (m *IAASModel) CloudRegion() string {
	return m.doc.CloudRegion
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
