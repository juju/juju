// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"io"
	"time"

	"github.com/juju/errors"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/resource"
)

// Resources describes the state functionality for resources.
type Resources interface {
	// ListResources returns the list of resources for the given service.
	ListResources(applicationID string) (resource.ServiceResources, error)

	// AddPendingResource adds the resource to the data store in a
	// "pending" state. It will stay pending (and unavailable) until
	// it is resolved. The returned ID is used to identify the pending
	// resources when resolving it.
	AddPendingResource(applicationID, userID string, chRes charmresource.Resource, r io.Reader) (string, error)

	// GetResource returns the identified resource.
	GetResource(applicationID, name string) (resource.Resource, error)

	// GetPendingResource returns the identified resource.
	GetPendingResource(applicationID, name, pendingID string) (resource.Resource, error)

	// SetResource adds the resource to blob storage and updates the metadata.
	SetResource(applicationID, userID string, res charmresource.Resource, r io.Reader) (resource.Resource, error)

	// UpdatePendingResource adds the resource to blob storage and updates the metadata.
	UpdatePendingResource(applicationID, pendingID, userID string, res charmresource.Resource, r io.Reader) (resource.Resource, error)

	// OpenResource returns the metadata for a resource and a reader for the resource.
	OpenResource(applicationID, name string) (resource.Resource, io.ReadCloser, error)

	// OpenResourceForUniter returns the metadata for a resource and a reader for the resource.
	OpenResourceForUniter(unit resource.Unit, name string) (resource.Resource, io.ReadCloser, error)

	// SetCharmStoreResources sets the "polled" resources for the
	// service to the provided values.
	SetCharmStoreResources(applicationID string, info []charmresource.Resource, lastPolled time.Time) error

	// TODO(ericsnow) Move this down to ResourcesPersistence.

	// NewResolvePendingResourcesOps generates mongo transaction operations
	// to set the identified resources as active.
	NewResolvePendingResourcesOps(applicationID string, pendingIDs map[string]string) ([]txn.Op, error)
}

var newResources func(Persistence, *State) Resources

// SetResourcesComponent registers the function that provide the state
// functionality related to resources.
func SetResourcesComponent(fn func(Persistence, *State) Resources) {
	newResources = fn
}

// Resources returns the resources functionality for the current state.
func (st *State) Resources() (Resources, error) {
	if newResources == nil {
		return nil, errors.NotSupportedf("resources")
	}

	persist := st.newPersistence()
	resources := newResources(persist, st)
	return resources, nil
}

// ResourcesPersistence exposes the resources persistence functionality
// needed by state.
type ResourcesPersistence interface {
	// NewRemoveUnitResourcesOps returns mgo transaction operations
	// that remove resource information specific to the unit from state.
	NewRemoveUnitResourcesOps(unitID string) ([]txn.Op, error)

	// NewRemoveResourcesOps returns mgo transaction operations that
	// remove all the service's resources from state.
	NewRemoveResourcesOps(applicationID string) ([]txn.Op, error)
}

var newResourcesPersistence func(Persistence) ResourcesPersistence

// SetResourcesPersistence registers the function that provides the
// state persistence functionality related to resources.
func SetResourcesPersistence(fn func(Persistence) ResourcesPersistence) {
	newResourcesPersistence = fn
}

// ResourcesPersistence returns the resources persistence functionality
// for the current state.
func (st *State) ResourcesPersistence() (ResourcesPersistence, error) {
	if newResourcesPersistence == nil {
		return nil, errors.NotSupportedf("resources")
	}

	base := st.newPersistence()
	persist := newResourcesPersistence(base)
	return persist, nil
}
