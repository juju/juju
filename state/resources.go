// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"io"
	"time"

	charmresource "github.com/juju/charm/v7/resource"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/resource"
)

// Resources describes the state functionality for resources.
type Resources interface {
	// ListResources returns the list of resources for the given application.
	ListResources(applicationID string) (resource.ApplicationResources, error)

	// ListPendingResources returns the list of pending resources for
	// the given application.
	ListPendingResources(applicationID string) ([]resource.Resource, error)

	// AddPendingResource adds the resource to the data store in a
	// "pending" state. It will stay pending (and unavailable) until
	// it is resolved. The returned ID is used to identify the pending
	// resources when resolving it.
	AddPendingResource(applicationID, userID string, chRes charmresource.Resource) (string, error)

	// GetResource returns the identified resource.
	GetResource(applicationID, name string) (resource.Resource, error)

	// GetPendingResource returns the identified resource.
	GetPendingResource(applicationID, name, pendingID string) (resource.Resource, error)

	// SetResource adds the resource to blob storage and updates the metadata.
	SetResource(applicationID, userID string, res charmresource.Resource, r io.Reader) (resource.Resource, error)

	// SetUnitResource sets the resource metadata for a specific unit.
	SetUnitResource(unitName, userID string, res charmresource.Resource) (resource.Resource, error)

	// UpdatePendingResource adds the resource to blob storage and updates the metadata.
	UpdatePendingResource(applicationID, pendingID, userID string, res charmresource.Resource, r io.Reader) (resource.Resource, error)

	// OpenResource returns the metadata for a resource and a reader for the resource.
	OpenResource(applicationID, name string) (resource.Resource, io.ReadCloser, error)

	// OpenResourceForUniter returns the metadata for a resource and a reader for the resource.
	OpenResourceForUniter(unit resource.Unit, name string) (resource.Resource, io.ReadCloser, error)

	// SetCharmStoreResources sets the "polled" resources for the
	// application to the provided values.
	SetCharmStoreResources(applicationID string, info []charmresource.Resource, lastPolled time.Time) error

	// RemovePendingAppResources removes any pending application-level
	// resources for the named application. This is used to clean up
	// resources for a failed application deployment.
	RemovePendingAppResources(applicationID string, pendingIDs map[string]string) error

	// TODO(ericsnow) Move this down to ResourcesPersistence.

	// NewResolvePendingResourcesOps generates mongo transaction operations
	// to set the identified resources as active.
	NewResolvePendingResourcesOps(applicationID string, pendingIDs map[string]string) ([]txn.Op, error)
}

// Resources returns the resources functionality for the current state.
func (st *State) Resources() (Resources, error) {
	persist := st.newPersistence()
	resources := NewResourceState(persist, st)
	return resources, nil
}

// ResourcesPersistence exposes the resources persistence functionality
// needed by state.
type ResourcesPersistence interface {
	// NewRemoveUnitResourcesOps returns mgo transaction operations
	// that remove resource information specific to the unit from state.
	NewRemoveUnitResourcesOps(unitID string) ([]txn.Op, error)

	// NewRemoveResourcesOps returns mgo transaction operations that
	// remove all the application's resources from state.
	NewRemoveResourcesOps(applicationID string) ([]txn.Op, error)
}

// ResourcesPersistence returns the resources persistence functionality
// for the current state.
func (st *State) ResourcesPersistence() (ResourcesPersistence, error) {
	base := st.newPersistence()
	persist := NewResourcePersistence(base)
	return persist, nil
}
