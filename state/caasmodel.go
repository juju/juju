// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
)

// CAASModel contains functionality that is specific to an
// Containers-As-A-Service (CAAS) model. It embeds a Model so that
// all generic Model functionality is also available.
type CAASModel struct {
	// TODO(caas) - this is all still messy until things shake out.
	*Model

	mb modelBackend
}

// CAASModel returns an Containers-As-A-Service (CAAS) model.
func (m *Model) CAASModel() (*CAASModel, error) {
	if m.TypeOld() != ModelTypeCAAS {
		return nil, errors.NotSupportedf("called CAASModel() on a non-CAAS Model")
	}
	return &CAASModel{
		Model: m,
		mb:    m.st,
	}, nil
}
