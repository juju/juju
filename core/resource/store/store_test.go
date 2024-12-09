// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store_test

import (
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	objectstoretesting "github.com/juju/juju/core/objectstore/testing"
	"github.com/juju/juju/core/resource/store"
	"github.com/juju/juju/core/resource/store/testing"
)

type resourcesStoreSuite struct {
	jujutesting.IsolationSuite
}

var _ = gc.Suite(&resourcesStoreSuite{})

func (*resourcesStoreSuite) TestFileResourceStoreID(c *gc.C) {
	expectedUUID := objectstoretesting.GenObjectStoreUUID(c)
	storeID, err := store.NewFileResourceID(expectedUUID)
	c.Assert(err, jc.ErrorIsNil)

	uuid, err := storeID.ObjectStoreUUID()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uuid, gc.Equals, expectedUUID)

	_, err = storeID.ContainerImageMetadataStoreID()
	c.Assert(err, gc.ErrorMatches, "container image metadata store ID not set")
}

func (*resourcesStoreSuite) TestContainerImageMetadataResourceID(c *gc.C) {
	expectedID := "test-id"
	storeID, err := store.NewContainerImageMetadataResourceID(expectedID)
	c.Assert(err, jc.ErrorIsNil)

	id, err := storeID.ContainerImageMetadataStoreID()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(id, gc.Equals, expectedID)

	_, err = storeID.ObjectStoreUUID()
	c.Assert(err, gc.ErrorMatches, "object store UUID not set")
}

func (*resourcesStoreSuite) TestIsZero(c *gc.C) {
	var id store.ID
	c.Assert(id.IsZero(), gc.Equals, true)

	id = testing.GenFileResourceStoreID(c, objectstoretesting.GenObjectStoreUUID(c))
	c.Assert(id.IsZero(), gc.Equals, false)

	id = testing.GenContainerImageMetadataResourceID(c, "test-id")
	c.Assert(id.IsZero(), gc.Equals, false)
}

func (*resourcesStoreSuite) TestIsObjectStoreUUID(c *gc.C) {
	var id store.ID
	c.Assert(id.IsObjectStoreUUID(), gc.Equals, false)

	id = testing.GenFileResourceStoreID(c, objectstoretesting.GenObjectStoreUUID(c))
	c.Assert(id.IsObjectStoreUUID(), gc.Equals, true)

	id = testing.GenContainerImageMetadataResourceID(c, "test-id")
	c.Assert(id.IsObjectStoreUUID(), gc.Equals, false)
}

func (*resourcesStoreSuite) TestIsContainerImageMetadataID(c *gc.C) {
	var id store.ID
	c.Assert(id.IsContainerImageMetadataID(), gc.Equals, false)

	id = testing.GenFileResourceStoreID(c, objectstoretesting.GenObjectStoreUUID(c))
	c.Assert(id.IsContainerImageMetadataID(), gc.Equals, false)

	id = testing.GenContainerImageMetadataResourceID(c, "test-id")
	c.Assert(id.IsContainerImageMetadataID(), gc.Equals, true)
}
