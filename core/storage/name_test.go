// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	
	"github.com/juju/juju/core/storage"
)

type storageNameSuite struct{}

var _ = gc.Suite(&storageNameSuite{})

func (s *storageNameSuite) TestStorageNameValidity(c *gc.C) {
	assertStorageNameValid(c, "shared-fs")
	assertStorageNameInvalid(c, "")
	assertStorageNameInvalid(c, "0")
}

func assertStorageNameValid(c *gc.C, name string) {
	result, err := storage.NewName(name)
	c.Assert(err, gc.IsNil)
	c.Assert(name, gc.Equals, result.String())
}

func assertStorageNameInvalid(c *gc.C, name string) {
	_, err := storage.NewName(name)
	c.Assert(err, jc.ErrorIs, storage.InvalidStorageName)
}

type storageIDSuite struct{}

var _ = gc.Suite(&storageIDSuite{})

func (s *storageIDSuite) TestStorageIDValidity(c *gc.C) {
	assertStorageIDValid(c, "shared-fs/0")
	assertStorageIDInvalid(c, "shared-fs")
	assertStorageIDInvalid(c, "")
	assertStorageIDInvalid(c, "0")
}

func assertStorageIDValid(c *gc.C, id string) {
	result, err := storage.NewID(id)
	c.Assert(err, gc.IsNil)
	c.Assert(id, gc.Equals, result.String())
}

func assertStorageIDInvalid(c *gc.C, id string) {
	_, err := storage.NewID(id)
	c.Assert(err, jc.ErrorIs, storage.InvalidStorageID)
}
