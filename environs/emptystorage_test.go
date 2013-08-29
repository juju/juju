// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	"io/ioutil"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/provider/dummy"
	"launchpad.net/juju-core/testing"
)

type EmptyStorageSuite struct{}

var _ = gc.Suite(&EmptyStorageSuite{})

func (s *EmptyStorageSuite) TestGet(c *gc.C) {
	f, err := environs.EmptyStorage.Get("anything")
	c.Assert(f, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `file "anything" not found`)
}

func (s *EmptyStorageSuite) TestURL(c *gc.C) {
	url, err := environs.EmptyStorage.URL("anything")
	c.Assert(url, gc.Equals, "")
	c.Assert(err, gc.ErrorMatches, `file "anything" not found`)
}

func (s *EmptyStorageSuite) TestList(c *gc.C) {
	names, err := environs.EmptyStorage.List("anything")
	c.Assert(names, gc.IsNil)
	c.Assert(err, gc.IsNil)
}

func (s *EmptyStorageSuite) TestPut(c *gc.C) {
	err := environs.EmptyStorage.Put("anything", nil, 0)
	c.Assert(err, gc.ErrorMatches, `cannot put file "anything" to empty storage`)
}

func (s *EmptyStorageSuite) TestRemove(c *gc.C) {
	err := environs.EmptyStorage.Remove("anything")
	c.Assert(err, gc.ErrorMatches, `cannot remove file "anything" from empty storage`)
}

func (s *EmptyStorageSuite) TestRemoveAll(c *gc.C) {
	err := environs.EmptyStorage.RemoveAll()
	c.Assert(err, gc.ErrorMatches, `cannot remove files from empty storage`)
}

type verifyStorageSuite struct{}

var _ = gc.Suite(&verifyStorageSuite{})

const existingEnv = `
environments:
    test:
        type: dummy
        state-server: false
        authorized-keys: i-am-a-key
`

func (s *verifyStorageSuite) TearDownTest(c *gc.C) {
	dummy.Reset()
}

func (s *verifyStorageSuite) TestVerifyStorage(c *gc.C) {
	defer testing.MakeFakeHome(c, existingEnv, "existing").Restore()

	environ, err := environs.NewFromName("test")
	c.Assert(err, gc.IsNil)
	storage := environ.Storage()
	err = environs.VerifyStorage(storage)
	c.Assert(err, gc.IsNil)
	reader, err := storage.Get("bootstrap-verify")
	c.Assert(err, gc.IsNil)
	defer reader.Close()
	contents, err := ioutil.ReadAll(reader)
	c.Assert(err, gc.IsNil)
	c.Check(string(contents), gc.Equals,
		"juju-core storage writing verified: ok\n")
}

func (s *verifyStorageSuite) TestVerifyStorageFails(c *gc.C) {
	defer testing.MakeFakeHome(c, existingEnv, "existing").Restore()

	environ, err := environs.NewFromName("test")
	c.Assert(err, gc.IsNil)
	storage := environ.Storage()
	someError := errors.Unauthorizedf("you shall not pass")
	dummy.Poison(storage, "bootstrap-verify", someError)
	err = environs.VerifyStorage(storage)
	c.Assert(err, gc.Equals, environs.VerifyStorageError)
}
