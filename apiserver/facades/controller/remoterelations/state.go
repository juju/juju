// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"gopkg.in/macaroon.v2"

	common "github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/state"
)

// RemoteRelationState provides the subset of global state required by the
// remote relations facade.
type RemoteRelationsState interface {
	common.Backend

	// WatchRemoteApplications returns a StringsWatcher that notifies of changes to
	// the lifecycles of the remote applications in the model.
	WatchRemoteApplications() state.StringsWatcher

	// WatchRemoteApplicationRelations returns a StringsWatcher that notifies of
	// changes to the life-cycles of relations involving the specified remote
	// application.
	WatchRemoteApplicationRelations(applicationName string) (state.StringsWatcher, error)

	// WatchRemoteRelations returns a StringsWatcher that notifies of changes to
	// the lifecycles of remote relations in the model.
	WatchRemoteRelations() state.StringsWatcher

	// RemoveRemoteEntity removes the specified entity from the remote entities collection.
	RemoveRemoteEntity(entity names.Tag) error

	// SaveMacaroon saves the given macaroon for the specified entity.
	SaveMacaroon(entity names.Tag, mac *macaroon.Macaroon) error

	// UpdateControllerForModel ensures that there is an external controller
	// record for the input info, associated with the input model ID.
	UpdateControllerForModel(controller crossmodel.ControllerInfo, modelUUID string) error
}

// TODO - CAAS(ericclaudejones): This should contain state alone, model will be
// removed once all relevant methods are moved from state to model.
type stateShim struct {
	common.Backend
	st *state.State
}

func (st stateShim) RemoveRemoteEntity(entity names.Tag) error {
	r := st.st.RemoteEntities()
	return r.RemoveRemoteEntity(entity)
}

func (st stateShim) GetToken(entity names.Tag) (string, error) {
	r := st.st.RemoteEntities()
	return r.GetToken(entity)
}

func (st stateShim) SaveMacaroon(entity names.Tag, mac *macaroon.Macaroon) error {
	r := st.st.RemoteEntities()
	return r.SaveMacaroon(entity, mac)
}

func (st stateShim) WatchRemoteApplications() state.StringsWatcher {
	return st.st.WatchRemoteApplications()
}

func (st stateShim) WatchRemoteRelations() state.StringsWatcher {
	return st.st.WatchRemoteRelations()
}

func (st stateShim) WatchRemoteApplicationRelations(applicationName string) (state.StringsWatcher, error) {
	a, err := st.st.RemoteApplication(applicationName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return a.WatchRelations(), nil
}

// UpdateControllerForModel (RemoteRelationsState) ensures
// that there is an external controller record for the input info,
// associated with the input model UUID.
// If the model UUID is associated with another external controller record,
// that record will be modified to remove it.
func (st stateShim) UpdateControllerForModel(controller crossmodel.ControllerInfo, modelUUID string) error {
	return errors.Trace(state.NewExternalControllers(st.st).SaveAndMoveModels(controller, modelUUID))
}
