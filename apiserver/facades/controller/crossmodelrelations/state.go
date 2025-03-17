// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations

import (
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	common "github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/state"
)

// CrossModelRelationsState provides the subset of global state required by the
// remote relations facade.
type CrossModelRelationsState interface {
	common.Backend

	// Model returns the model entity.
	Model() (Model, error)

	// AddOfferConnection creates a new offer connection record, which records details about a
	// relation made from a remote model to an offer in the local model.
	AddOfferConnection(common.AddOfferConnectionParams) (common.OfferConnection, error)

	// IsMigrationActive returns true if the current model is
	// in the process of being migrated to another controller.
	IsMigrationActive() (bool, error)
}

// TODO - CAAS(ericclaudejones): This should contain state alone, model will be
// removed once all relevant methods are moved from state to model.
type stateShim struct {
	common.Backend
	st *state.State
}

func (st stateShim) ApplicationOfferForUUID(offerUUID string) (*crossmodel.ApplicationOffer, error) {
	return nil, errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func (st stateShim) AddOfferConnection(arg common.AddOfferConnectionParams) (common.OfferConnection, error) {
	return nil, errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func (st stateShim) OfferConnectionForRelation(relationKey string) (common.OfferConnection, error) {
	return nil, errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

// IsMigrationActive returns true if the current model is
// in the process of being migrated to another controller.
func (st stateShim) IsMigrationActive() (bool, error) {
	migrating, err := st.st.IsMigrationActive()
	return migrating, errors.Trace(err)
}

type Model interface {
	Name() string
	Owner() names.UserTag
}

func (st stateShim) Model() (Model, error) {
	return st.st.Model()
}
