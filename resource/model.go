// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

// TODO(ericsnow) Move ModelResource to an internal package?

// ModelResource represents the full information about a resource
// in a Juju model.
type ModelResource struct {
	// ID is the model-defined ID for the resource.
	ID string

	// ServiceID identifies the service for the resource.
	ServiceID string

	// Resource is the general info for the resource.
	Resource Resource
}
