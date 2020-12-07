// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workers

import (
	"time"

	"github.com/juju/charm/v8/resource"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
)

// DataStore exposes the functionality of Juju state needed here.
type DataStore interface {
	// SetCharmStoreResources sets the "polled from the charm store"
	// resources for the application to the provided values.
	SetCharmStoreResources(applicationID string, info []resource.Resource, lastPolled time.Time) error
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

// TODO(benhoyt) - get rid of this whole mess, and just call SetCharmStoreResources
// directly from updateLatestRevisions()

// HandleLatest implements apiserver/facades/controller/charmrevisionupdater.LatestCharmHandler
// by storing the charm's resources in state.
func (handler LatestCharmHandler) HandleLatest(applicationID names.ApplicationTag, resources []resource.Resource, timestamp time.Time) error {
	if err := handler.store.SetCharmStoreResources(applicationID.Id(), resources, timestamp); err != nil {
		return errors.Trace(err)
	}
	return nil
}
