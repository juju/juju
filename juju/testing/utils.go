// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/tc"

	coreobjectstore "github.com/juju/juju/core/objectstore"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/objectstore"
	objectstoretesting "github.com/juju/juju/internal/objectstore/testing"
)

// NewObjectStore creates a new object store for testing.
// This uses the memory metadata service.
func NewObjectStore(c *tc.C, modelUUID string) coreobjectstore.ObjectStore {
	return NewObjectStoreWithMetadataService(c, modelUUID, objectstoretesting.MemoryMetadataService())
}

// NewObjectStoreWithMetadataService creates a new object store for testing.
func NewObjectStoreWithMetadataService(c *tc.C, modelUUID string, metadataService objectstore.MetadataService) coreobjectstore.ObjectStore {
	store, err := objectstore.ObjectStoreFactory(
		c.Context(),
		objectstore.DefaultBackendType(),
		modelUUID,
		objectstore.WithRootDir(c.MkDir()),
		objectstore.WithLogger(loggertesting.WrapCheckLog(c)),

		// TODO (stickupkid): Swap this over to the real metadata service
		// when all facades are moved across.
		objectstore.WithMetadataService(metadataService),
		objectstore.WithClaimer(objectstoretesting.MemoryClaimer()),
	)
	c.Assert(err, tc.ErrorIsNil)
	return store
}
