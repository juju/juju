// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

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
	RemoveRemoteEntity(sourceModel names.ModelTag, entity names.Tag) error

	// GetToken returns the token associated with the entity with the given tag
	// and model.
	GetToken(names.ModelTag, names.Tag) (string, error)
}

type stateShim struct {
	common.Backend
	st *state.State
}

func (st stateShim) RemoveRemoteEntity(model names.ModelTag, entity names.Tag) error {
	r := st.st.RemoteEntities()
	return r.RemoveRemoteEntity(model, entity)
}

func (st stateShim) GetToken(model names.ModelTag, entity names.Tag) (string, error) {
	r := st.st.RemoteEntities()
	return r.GetToken(model, entity)
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
