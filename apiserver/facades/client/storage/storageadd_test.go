// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"testing"

	"github.com/juju/tc"
)

type storageAddSuite struct {
	baseStorageSuite
}

func TestStorageAddSuite(t *testing.T) {
	tc.Run(t, &storageAddSuite{})
}

func (s *storageAddSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing the following tests:
- TestStorageAddEmpty
- TestStorageAddUnit
- TestStorageAddUnitBlocked
- TestStorageAddUnitDestroyIgnored
- TestStorageAddUnitInvalidName
- TestStorageAddUnitStateError
- TestStorageAddUnitResultOrder
- TestStorageAddUnitTags
- TestStorageAddUnitNotFoundErr
`)
}
