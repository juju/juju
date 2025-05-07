// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/core/storage"
)

type storageNameSuite struct{}

var _ = tc.Suite(&storageNameSuite{})

func (s *storageNameSuite) TestStorageNameValidity(c *tc.C) {
	assertStorageNameValid(c, "shared-fs")
	assertStorageNameInvalid(c, "")
	assertStorageNameInvalid(c, "0")
}

func assertStorageNameValid(c *tc.C, name string) {
	result, err := storage.ParseName(name)
	c.Assert(err, tc.IsNil)
	c.Assert(name, tc.Equals, result.String())
}

func assertStorageNameInvalid(c *tc.C, name string) {
	_, err := storage.ParseName(name)
	c.Assert(err, jc.ErrorIs, storage.InvalidStorageName)
}

type storageIDSuite struct{}

var _ = tc.Suite(&storageIDSuite{})

func (s *storageIDSuite) TestStorageIDValidity(c *tc.C) {
	assertStorageIDValid(c, "shared-fs/0")
	assertStorageIDInvalid(c, "shared-fs")
	assertStorageIDInvalid(c, "")
	assertStorageIDInvalid(c, "0")
}

func assertStorageIDValid(c *tc.C, id string) {
	result, err := storage.ParseID(id)
	c.Assert(err, tc.IsNil)
	c.Assert(id, tc.Equals, result.String())
}

func assertStorageIDInvalid(c *tc.C, id string) {
	_, err := storage.ParseID(id)
	c.Assert(err, jc.ErrorIs, storage.InvalidStorageID)
}

func (s *storageIDSuite) TestMakeID(c *tc.C) {
	id := storage.MakeID("foo", 666)
	c.Assert(id, tc.Equals, storage.ID("foo/666"))
}
