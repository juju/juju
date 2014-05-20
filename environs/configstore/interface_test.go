// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package configstore_test

import (
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/configstore"
	"launchpad.net/juju-core/testing"
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
	info, err := store.CreateInfo("someenv")
	c.Assert(err, gc.IsNil)
	c.Assert(info.APIEndpoint(), gc.DeepEquals, configstore.APIEndpoint{})
	c.Assert(info.APICredentials(), gc.DeepEquals, configstore.APICredentials{})
	c.Assert(info.Initialized(), jc.IsFalse)

	// Check that we can't create it twice.
	info, err = store.CreateInfo("someenv")
	c.Assert(err, gc.Equals, configstore.ErrEnvironInfoAlreadyExists)
	c.Assert(info, gc.IsNil)

	// Check that we can read it again.
	info, err = store.ReadInfo("someenv")
	c.Assert(err, gc.IsNil)
	c.Assert(info.Initialized(), jc.IsFalse)
}

func (s *interfaceSuite) TestSetAPIEndpointAndCredentials(c *gc.C) {
	store := s.NewStore(c)

	info, err := store.CreateInfo("someenv")
	c.Assert(err, gc.IsNil)

	expectEndpoint := configstore.APIEndpoint{
		Addresses: []string{"example.com"},
		CACert:    "a cert",
	}
	info.SetAPIEndpoint(expectEndpoint)
	c.Assert(info.APIEndpoint(), gc.DeepEquals, expectEndpoint)

	expectCreds := configstore.APICredentials{
		User:     "foobie",
		Password: "bletch",
	}
	info.SetAPICredentials(expectCreds)
	c.Assert(info.APICredentials(), gc.DeepEquals, expectCreds)
}

func (s *interfaceSuite) TestWrite(c *gc.C) {
	store := s.NewStore(c)

	// Create the info.
	info, err := store.CreateInfo("someenv")
	c.Assert(err, gc.IsNil)

	// Set it up with some actual data and write it out.
	expectCreds := configstore.APICredentials{
		User:     "foobie",
		Password: "bletch",
	}
	info.SetAPICredentials(expectCreds)

	expectEndpoint := configstore.APIEndpoint{
		Addresses: []string{"example.com"},
		CACert:    "a cert",
	}
	info.SetAPIEndpoint(expectEndpoint)

	err = info.Write()
	c.Assert(err, gc.IsNil)
	c.Assert(info.Initialized(), jc.IsTrue)

	// Check we can read the information back
	info, err = store.ReadInfo("someenv")
	c.Assert(err, gc.IsNil)
	c.Assert(info.APICredentials(), gc.DeepEquals, expectCreds)
	c.Assert(info.APIEndpoint(), gc.DeepEquals, expectEndpoint)

	// Change the information and write it again.
	expectCreds.User = "arble"
	info.SetAPICredentials(expectCreds)
	err = info.Write()
	c.Assert(err, gc.IsNil)

	// Check we can read the information back
	info, err = store.ReadInfo("someenv")
	c.Assert(err, gc.IsNil)
	c.Assert(info.APICredentials(), gc.DeepEquals, expectCreds)
}

func (s *interfaceSuite) TestDestroy(c *gc.C) {
	store := s.NewStore(c)

	info, err := store.CreateInfo("someenv")
	c.Assert(err, gc.IsNil)

	err = info.Destroy()
	c.Assert(err, gc.IsNil)

	err = info.Destroy()
	c.Assert(err, gc.ErrorMatches, "environment info has already been removed")

	info, err = store.CreateInfo("someenv")
	c.Assert(err, gc.IsNil)
}

func (s *interfaceSuite) TestNoBleedThrough(c *gc.C) {
	store := s.NewStore(c)

	info, err := store.CreateInfo("someenv")
	c.Assert(err, gc.IsNil)

	info.SetAPICredentials(configstore.APICredentials{User: "foo"})
	info.SetAPIEndpoint(configstore.APIEndpoint{CACert: "blah"})
	attrs := map[string]interface{}{"foo": "bar"}
	info.SetBootstrapConfig(attrs)

	info1, err := store.ReadInfo("someenv")
	c.Assert(err, gc.IsNil)
	c.Assert(info1.Initialized(), jc.IsFalse)
	c.Assert(info1.APICredentials(), gc.DeepEquals, configstore.APICredentials{})
	c.Assert(info1.APIEndpoint(), gc.DeepEquals, configstore.APIEndpoint{})
	c.Assert(info1.BootstrapConfig(), gc.HasLen, 0)

	err = info.Write()
	c.Assert(err, gc.IsNil)

	attrs["foo"] = "different"

	info1, err = store.ReadInfo("someenv")
	c.Assert(err, gc.IsNil)
	c.Assert(info1.Initialized(), jc.IsTrue)
	c.Assert(info1.BootstrapConfig(), gc.DeepEquals, map[string]interface{}{"foo": "bar"})
}

func (s *interfaceSuite) TestSetBootstrapConfigPanicsWhenNotCreated(c *gc.C) {
	store := s.NewStore(c)

	info, err := store.CreateInfo("someenv")
	c.Assert(err, gc.IsNil)
	info.SetBootstrapConfig(map[string]interface{}{"foo": "bar"})
	err = info.Write()
	c.Assert(err, gc.IsNil)

	info, err = store.ReadInfo("someenv")
	c.Assert(err, gc.IsNil)
	c.Assert(func() { info.SetBootstrapConfig(nil) }, gc.PanicMatches, "bootstrap config set on environment info that has not just been created")
}
