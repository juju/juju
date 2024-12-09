// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store

import (
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	objectstoretesting "github.com/juju/juju/core/objectstore/testing"
)

type resourcesStoreSuite struct {
	jujutesting.IsolationSuite
}

var _ = gc.Suite(&resourcesStoreSuite{})

func (*resourcesStoreSuite) TestFileResourceStoreID(c *gc.C) {
	expectedUUID := objectstoretesting.GenObjectStoreUUID(c)
	storeID, err := NewFileResourceID(expectedUUID)
	c.Assert(err, jc.ErrorIsNil)

	uuid, err := storeID.ObjectStoreUUID()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uuid, gc.Equals, expectedUUID)

	_, err = storeID.ContainerImageMetadataStoreID()
	c.Assert(err, gc.ErrorMatches, "container image metadata store ID is empty")
}

func (*resourcesStoreSuite) TestContainerImageMetadataResourceID(c *gc.C) {
	expectedID := "test-id"
	storeID, err := NewContainerImageMetadataResourceID(expectedID)
	c.Assert(err, jc.ErrorIsNil)

	id, err := storeID.ContainerImageMetadataStoreID()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(id, gc.Equals, expectedID)

	_, err = storeID.ObjectStoreUUID()
	c.Assert(err, gc.ErrorMatches, "object store UUID is empty")
}

func (*resourcesStoreSuite) TestIsZero(c *gc.C) {
	var id ID
	c.Assert(id.IsZero(), gc.Equals, true)

	id, err := NewFileResourceID(objectstoretesting.GenObjectStoreUUID(c))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(id.IsZero(), gc.Equals, false)

	id, err = NewContainerImageMetadataResourceID("test-id")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(id.IsZero(), gc.Equals, false)
}
