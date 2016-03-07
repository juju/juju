// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package configstore_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/testing"
)

// interfaceSuite defines a set of tests on a ConfigStorage
// implementation, independent of the implementation itself.
// The NewStore field must be set up to return a ConfigStorage
// instance of the type to be tested.
type interfaceSuite struct {
	testing.BaseSuite
	NewStore func(c *gc.C) configstore.Storage
}

func (s *interfaceSuite) TestCreate(c *gc.C) {
	store := s.NewStore(c)
	info := store.CreateInfo("uuid", "somemodel")
	c.Assert(info.Initialized(), jc.IsFalse)

	// The info isn't written until you call Write
	_, err := store.ReadInfo("somemodel")
	c.Assert(err, gc.ErrorMatches, `model "somemodel" not found`)

	err = info.Write()
	c.Assert(err, jc.ErrorIsNil)

	// Check that we can read it again.
	info, err = store.ReadInfo("somemodel")
	c.Assert(err, jc.ErrorIsNil)

	// Now that it exists, we cannot write a newly created info again.
	info = store.CreateInfo("uuid", "somemodel")
	err = info.Write()
	c.Assert(errors.Cause(err), gc.Equals, configstore.ErrEnvironInfoAlreadyExists)
}

func (s *interfaceSuite) createInitialisedEnvironment(c *gc.C, store configstore.Storage, envName, modelUUID, serverUUID string) {
	info := store.CreateInfo(serverUUID, envName)
	err := info.Write()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *interfaceSuite) TestWrite(c *gc.C) {
	store := s.NewStore(c)

	// Create the info.
	info := store.CreateInfo("uuid", "somemodel")
	err := info.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.Initialized(), jc.IsTrue)

	// Check we can read the information back
	info, err = store.ReadInfo("somemodel")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *interfaceSuite) TestDestroy(c *gc.C) {
	store := s.NewStore(c)

	info := store.CreateInfo("uuid", "somemodel")
	// Destroying something that hasn't been written is fine.
	err := info.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	err = info.Write()
	c.Assert(err, jc.ErrorIsNil)

	err = info.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	err = info.Destroy()
	c.Assert(err, gc.ErrorMatches, "model info has already been removed")
}

func (s *interfaceSuite) TestNoBleedThrough(c *gc.C) {
	store := s.NewStore(c)

	info := store.CreateInfo("uuid", "somemodel")

	attrs := map[string]interface{}{"foo": "bar"}
	info.SetBootstrapConfig(attrs)

	err := info.Write()
	c.Assert(err, jc.ErrorIsNil)

	attrs["foo"] = "different"

	info1, err := store.ReadInfo("somemodel")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info1.Initialized(), jc.IsTrue)
	c.Assert(info1.BootstrapConfig(), gc.DeepEquals, map[string]interface{}{"foo": "bar"})
}

func (s *interfaceSuite) TestSetBootstrapConfigPanicsWhenNotCreated(c *gc.C) {
	store := s.NewStore(c)

	info := store.CreateInfo("uuid", "somemodel")
	info.SetBootstrapConfig(map[string]interface{}{"foo": "bar"})
	err := info.Write()
	c.Assert(err, jc.ErrorIsNil)

	info, err = store.ReadInfo("somemodel")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(func() { info.SetBootstrapConfig(nil) }, gc.PanicMatches, "bootstrap config set on model info that has not just been created")
}
