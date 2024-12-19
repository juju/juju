// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package services

import (
	"context"
	"io"

	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/domain/application/charm"
	internalcharm "github.com/juju/juju/internal/charm"
)

// ApplicationService provides access to application related operations, this
// includes charms, units and resources.
type ApplicationService interface {
	// GetCharmID returns a charm ID by name. It returns an error.CharmNotFound
	// if the charm can not be found by the name.
	// This can also be used as a cheap way to see if a charm exists without
	// needing to load the charm metadata.
	GetCharmID(ctx context.Context, args charm.GetCharmArgs) (corecharm.ID, error)

	// GetCharm returns the charm metadata for the given charm ID.
	// It returns an error.CharmNotFound if the charm can not be found by the
	// ID.
	GetCharm(ctx context.Context, id corecharm.ID) (internalcharm.Charm, charm.CharmLocator, error)
}

// Storage describes an API for storing and deleting blobs.
type Storage interface {
	// Put stores data from reader at path, namespaced to the model.
	Put(context.Context, string, io.Reader, int64) (objectstore.UUID, error)
	// Remove removes data at path, namespaced to the model.
	Remove(context.Context, string) error
}
