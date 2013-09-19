// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package configstore_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	jc "launchpad.net/juju-core/testing/checkers"
)

// interfaceSuite defines a set of tests on a ConfigStorage
// implementation, independent of the implementation itself.
// The NewStore field must be set up to return a ConfigStorage
// instance of the type to be tested.
type interfaceSuite struct {
	NewStore func(c *gc.C) environs.ConfigStorage
}

func (s *interfaceSuite) TestNew(c *gc.C) {
	s.NewStore(c)
}

func (s *interfaceSuite) TestCreate(c *gc.C) {
	store := s.NewStore(c)
	info, err := store.CreateInfo("someenv")
	c.Assert(err, gc.IsNil)
	c.Assert(info.APIEndpoint(), gc.DeepEquals, environs.APIEndpoint{})
	c.Assert(info.APICredentials(), gc.DeepEquals, environs.APICredentials{})
	c.Assert(info.Initialized(), jc.IsFalse)

	// Check that we can't create it twice.
	info, err = store.CreateInfo("someenv")
	c.Assert(err, gc.Equals, environs.ErrEnvironInfoAlreadyExists)
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

	expectEndpoint := environs.APIEndpoint{
		Addresses: []string{"example.com"},
		CACert:    "a cert",
	}
	info.SetAPIEndpoint(expectEndpoint)
	c.Assert(info.APIEndpoint(), gc.DeepEquals, expectEndpoint)

	expectCreds := environs.APICredentials{
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
	expectCreds := environs.APICredentials{
		User:     "foobie",
		Password: "bletch",
	}
	info.SetAPICredentials(expectCreds)

	expectEndpoint := environs.APIEndpoint{
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

	info.SetAPICredentials(environs.APICredentials{User: "foo"})
	info.SetAPIEndpoint(environs.APIEndpoint{CACert: "blah"})

	info1, err := store.ReadInfo("someenv")
	c.Assert(err, gc.IsNil)
	c.Assert(info1.Initialized(), jc.IsFalse)
	c.Assert(info1.APICredentials(), gc.DeepEquals, environs.APICredentials{})
	c.Assert(info1.APIEndpoint(), gc.DeepEquals, environs.APIEndpoint{})
}
