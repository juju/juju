// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workers

import (
	"time"

	charmresource "github.com/juju/charm/v7/resource"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/charmstore"
)

// DataStore exposes the functionality of Juju state needed here.
type DataStore interface {
	// SetCharmStoreResources sets the "polled from the charm store"
	// resources for the application to the provided values.
	SetCharmStoreResources(applicationID string, info []charmresource.Resource, lastPolled time.Time) error
}

// LatestCharmHandler implements apiserver/facades/controller/charmrevisionupdater.LatestCharmHandler.
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

// HandleLatest implements apiserver/facades/controller/charmrevisionupdater.LatestCharmHandler
// by storing the charm's resources in state.
func (handler LatestCharmHandler) HandleLatest(applicationID names.ApplicationTag, info charmstore.CharmInfo) error {
	if err := handler.store.SetCharmStoreResources(applicationID.Id(), info.LatestResources, info.Timestamp); err != nil {
		return errors.Trace(err)
	}
	return nil
}
