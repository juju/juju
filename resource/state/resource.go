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
}

type resourceStorage interface {
	// Put stores the content of the reader into the storage.
	Put(hash string, r io.Reader, length int64) error
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
