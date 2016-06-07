// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workers

import (
	"time"

	"github.com/juju/errors"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/charmstore"
)

// DataStore exposes the functionality of Juju state needed here.
type DataStore interface {
	// SetCharmStoreResources sets the "polled from the charm store"
	// resources for the application to the provided values.
	SetCharmStoreResources(applicationID string, info []charmresource.Resource, lastPolled time.Time) error
}

// LatestCharmHandler implements apiserver/charmrevisionupdater.LatestCharmHandler.
type LatestCharmHandler struct {
	store DataStore
}

// NewLatestCharmHandler returns a LatestCharmHandler that uses the
// given data store.
func NewLatestCharmHandler(store DataStore) *LatestCharmHandler {
	return &LatestCharmHandler{
		store: store,
	}
}

// HandleLatest implements apiserver/charmrevisionupdater.LatestCharmHandler
// by storing the charm's resources in state.
func (handler LatestCharmHandler) HandleLatest(applicationID names.ApplicationTag, info charmstore.CharmInfo) error {
	if err := handler.store.SetCharmStoreResources(applicationID.Id(), info.LatestResources, info.Timestamp); err != nil {
		return errors.Trace(err)
	}
	return nil
}
