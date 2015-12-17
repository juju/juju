// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"io"

	"github.com/juju/errors"

	"github.com/juju/juju/resource"
)

type resourcePersistence interface {
	// ListResources returns the resource data for the given service ID.
	ListResources(serviceID string) ([]resource.Resource, error)

	// SetStagedResource adds the resource in a separate staging area
	// if the resource isn't already staged. If the resource already
	// exists then it is treated as unavailable as long as the new one
	// is staged.
	SetStagedResource(serviceID string, res resource.Resource) error

	// UnstageResource ensures that the resource is removed
	// from the staging area. If it isn't in the staging area
	// then this is a noop.
	UnstageResource(serviceID, resName string) error

	// SetResource stores the resource info. If the resource
	// is already staged then it is unstaged, unless the staged
	// resource is different. In that case the request will fail.
	SetResource(serviceID string, res resource.Resource) error
}

type resourceStorage interface {
	// Put stores the content of the reader into the storage.
	Put(hash string, r io.Reader, length int64) error

	// Delete removes the identified data from the storage.
	Delete(hash string) error
}

type resourceState struct {
	persist resourcePersistence
	storage resourceStorage
}

// ListResources returns the resource data for the given service ID.
func (st resourceState) ListResources(serviceID string) ([]resource.Resource, error) {
	resources, err := st.persist.ListResources(serviceID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return resources, nil
}
