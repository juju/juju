// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

// TODO(ericsnow) Move ModelResource to an internal package?

// ModelResource represents the full information about a resource
// in a Juju model.
type ModelResource struct {
	// ID uniquely identifies a resource-service pair within the model.
	// Note that the model ignores pending resources (those with a
	// pending ID) except for in a few clearly pending-related places.
	ID string

	// PendingID identifies that this resource is pending and
	// distinguishes it from other pending resources with the same model
	// ID (and from the active resource).
	PendingID string

	// ServiceID identifies the service for the resource.
	ServiceID string

	// Resource is the general info for the resource.
	Resource Resource

	// TODO(ericsnow) Use StoragePath for the directory path too?

	// StoragePath is the path to where the resource content is stored.
	StoragePath string
}
