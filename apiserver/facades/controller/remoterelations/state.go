// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon.v1"

	common "github.com/juju/juju/apiserver/common/crossmodel"
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
	// changes to the lifecycles of relations involving the specified remote
	// application.
	WatchRemoteApplicationRelations(applicationName string) (state.StringsWatcher, error)

	// WatchRemoteRelations returns a StringsWatcher that notifies of changes to
	// the lifecycles of remote relations in the model.
	WatchRemoteRelations() state.StringsWatcher

	// RemoveRemoteEntity removes the specified entity from the remote entities collection.
	RemoveRemoteEntity(entity names.Tag) error

	// GetToken returns the token associated with the entity with the given tag.
	GetToken(names.Tag) (string, error)

	// SaveMacaroon saves the given macaroon for the specified entity.
	SaveMacaroon(entity names.Tag, mac *macaroon.Macaroon) error
}

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
