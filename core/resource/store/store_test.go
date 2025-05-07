// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store_test

import (
	"github.com/juju/tc"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"

	objectstoretesting "github.com/juju/juju/core/objectstore/testing"
	"github.com/juju/juju/core/resource/store"
	"github.com/juju/juju/core/resource/store/testing"
)

type resourcesStoreSuite struct {
	jujutesting.IsolationSuite
}

var _ = tc.Suite(&resourcesStoreSuite{})

func (*resourcesStoreSuite) TestFileResourceStoreID(c *tc.C) {
	expectedUUID := objectstoretesting.GenObjectStoreUUID(c)
	storeID, err := store.NewFileResourceID(expectedUUID)
	c.Assert(err, jc.ErrorIsNil)

	uuid, err := storeID.ObjectStoreUUID()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uuid, tc.Equals, expectedUUID)

	_, err = storeID.ContainerImageMetadataStoreID()
	c.Assert(err, tc.ErrorMatches, "container image metadata store ID not set")
}

func (*resourcesStoreSuite) TestContainerImageMetadataResourceID(c *tc.C) {
	expectedID := "test-id"
	storeID, err := store.NewContainerImageMetadataResourceID(expectedID)
	c.Assert(err, jc.ErrorIsNil)

	id, err := storeID.ContainerImageMetadataStoreID()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(id, tc.Equals, expectedID)

	_, err = storeID.ObjectStoreUUID()
	c.Assert(err, tc.ErrorMatches, "object store UUID not set")
}

func (*resourcesStoreSuite) TestIsZero(c *tc.C) {
	var id store.ID
	c.Assert(id.IsZero(), tc.Equals, true)

	id = testing.GenFileResourceStoreID(c, objectstoretesting.GenObjectStoreUUID(c))
	c.Assert(id.IsZero(), tc.Equals, false)

	id = testing.GenContainerImageMetadataResourceID(c, "test-id")
	c.Assert(id.IsZero(), tc.Equals, false)
}

func (*resourcesStoreSuite) TestIsObjectStoreUUID(c *tc.C) {
	var id store.ID
	c.Assert(id.IsObjectStoreUUID(), tc.Equals, false)

	id = testing.GenFileResourceStoreID(c, objectstoretesting.GenObjectStoreUUID(c))
	c.Assert(id.IsObjectStoreUUID(), tc.Equals, true)

	id = testing.GenContainerImageMetadataResourceID(c, "test-id")
	c.Assert(id.IsObjectStoreUUID(), tc.Equals, false)
}

func (*resourcesStoreSuite) TestIsContainerImageMetadataID(c *tc.C) {
	var id store.ID
	c.Assert(id.IsContainerImageMetadataID(), tc.Equals, false)

	id = testing.GenFileResourceStoreID(c, objectstoretesting.GenObjectStoreUUID(c))
	c.Assert(id.IsContainerImageMetadataID(), tc.Equals, false)

	id = testing.GenContainerImageMetadataResourceID(c, "test-id")
	c.Assert(id.IsContainerImageMetadataID(), tc.Equals, true)
}
