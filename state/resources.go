// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"

	"github.com/juju/juju/resource"
)

// Resources describes the state functionality for resources.
type Resources interface {
	// ListResources returns the list of resources for the given service.
	ListResources(serviceID string) ([]resource.Resource, error)
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
