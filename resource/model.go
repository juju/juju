// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

// TODO(ericsnow) Move ModelResource to an internal package?

// ModelResource represents the full information about a resource
// in a Juju model.
type ModelResource struct {
	// ID is the model-defined ID for the resource.
	ID string

	// PendingID is the token for a pending resource, if any.
	PendingID string

	// ServiceID identifies the service for the resource.
	ServiceID string

	// Resource is the general info for the resource.
	Resource Resource

	// TODO(ericsnow) Use StoragePath for the directory path too?

	// StoragePath is the path to where the resource content is stored.
	StoragePath string
}
