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
	ListResources(serviceID string) (resource.ServiceResources, error)

	// AddPendingResource adds the resource to the data store in a
	// "pending" state. It will stay pending (and unavailable) until
	// it is resolved. The returned ID is used to identify the pending
	// resources when resolving it.
	AddPendingResource(serviceID, userID string, chRes charmresource.Resource, r io.Reader) (string, error)

	// GetResource returns the identified resource.
	GetResource(serviceID, name string) (resource.Resource, error)

	// GetPendingResource returns the identified resource.
	GetPendingResource(serviceID, name, pendingID string) (resource.Resource, error)

	// SetResource adds the resource to blob storage and updates the metadata.
	SetResource(serviceID, userID string, res charmresource.Resource, r io.Reader) (resource.Resource, error)

	// UpdatePendingResource adds the resource to blob storage and updates the metadata.
	UpdatePendingResource(serviceID, pendingID, userID string, res charmresource.Resource, r io.Reader) (resource.Resource, error)

	// OpenResource returns the metadata for a resource and a reader for the resource.
	OpenResource(serviceID, name string) (resource.Resource, io.ReadCloser, error)

	// OpenResourceForUniter returns the metadata for a resource and a reader for the resource.
	OpenResourceForUniter(unit resource.Unit, name string) (resource.Resource, io.ReadCloser, error)

	// SetCharmStoreResources sets the "polled" resources for the
	// service to the provided values.
	SetCharmStoreResources(serviceID string, info []charmresource.Resource, lastPolled time.Time) error

	// NewResolvePendingResourcesOps generates mongo transaction operations
	// to set the identified resources as active.
	NewResolvePendingResourcesOps(serviceID string, pendingIDs map[string]string) ([]txn.Op, error)
}

var newResources func(Persistence) Resources

// SetResourcesComponent registers the function that provide the state
// functionality related to resources.
func SetResourcesComponent(fn func(Persistence) Resources) {
	newResources = fn
}

// Resources returns the resources functionality for the current state.
func (st *State) Resources() (Resources, error) {
	if newResources == nil {
		return nil, errors.Errorf("resources not supported")
	}

	persist := st.newPersistence()
	resources := newResources(persist)
	return resources, nil
}
