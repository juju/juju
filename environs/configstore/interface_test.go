// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package configstore_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/feature"
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
	info := store.CreateInfo("someenv")
	c.Assert(info.APIEndpoint(), gc.DeepEquals, configstore.APIEndpoint{})
	c.Assert(info.APICredentials(), gc.DeepEquals, configstore.APICredentials{})
	c.Assert(info.Initialized(), jc.IsFalse)

	// The info isn't written until you call Write
	_, err := store.ReadInfo("someenv")
	c.Assert(err, gc.ErrorMatches, `environment "someenv" not found`)

	err = info.Write()
	c.Assert(err, jc.ErrorIsNil)

	// Check that we can read it again.
	info, err = store.ReadInfo("someenv")
	c.Assert(err, jc.ErrorIsNil)

	// Now that it exists, we cannot write a newly created info again.
	info = store.CreateInfo("someenv")
	err = info.Write()
	c.Assert(errors.Cause(err), gc.Equals, configstore.ErrEnvironInfoAlreadyExists)
}

func (s *interfaceSuite) createInitialisedEnvironment(c *gc.C, store configstore.Storage, envName, envUUID, serverUUID string) {
	info := store.CreateInfo(envName)
	info.SetAPIEndpoint(configstore.APIEndpoint{
		Addresses:   []string{"localhost"},
		CACert:      testing.CACert,
		EnvironUUID: envUUID,
		ServerUUID:  serverUUID,
	})
	err := info.Write()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *interfaceSuite) TestList(c *gc.C) {
	store := s.NewStore(c)
	s.createInitialisedEnvironment(c, store, "enva", "enva-uuid", "")
	s.createInitialisedEnvironment(c, store, "envb", "envb-uuid", "")

	s.SetFeatureFlags(feature.JES)
	s.createInitialisedEnvironment(c, store, "envc", "envc-uuid", "envc-uuid")
	s.createInitialisedEnvironment(c, store, "system", "", "system-uuid")

	environs, err := store.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(environs, jc.SameContents, []string{"enva", "envb", "envc"})
}

func (s *interfaceSuite) TestListSystems(c *gc.C) {
	store := s.NewStore(c)
	s.createInitialisedEnvironment(c, store, "enva", "enva-uuid", "")
	s.createInitialisedEnvironment(c, store, "envb", "envb-uuid", "")

	s.SetFeatureFlags(feature.JES)
	s.createInitialisedEnvironment(c, store, "envc", "envc-uuid", "envc-uuid")
	s.createInitialisedEnvironment(c, store, "envd", "envd-uuid", "envc-uuid")
	s.createInitialisedEnvironment(c, store, "system", "", "system-uuid")

	environs, err := store.ListSystems()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(environs, jc.SameContents, []string{"enva", "envb", "envc", "system"})
}

func (s *interfaceSuite) TestSetAPIEndpointAndCredentials(c *gc.C) {
	store := s.NewStore(c)

	info := store.CreateInfo("someenv")

	expectEndpoint := configstore.APIEndpoint{
		Addresses:   []string{"0.1.2.3"},
		Hostnames:   []string{"example.com"},
		CACert:      "a cert",
		EnvironUUID: "dead-beef",
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
	info := store.CreateInfo("someenv")

	// Set it up with some actual data and write it out.
	expectCreds := configstore.APICredentials{
		User:     "foobie",
		Password: "bletch",
	}
	info.SetAPICredentials(expectCreds)

	expectEndpoint := configstore.APIEndpoint{
		Addresses:   []string{"0.1.2.3"},
		Hostnames:   []string{"example.invalid"},
		CACert:      "a cert",
		EnvironUUID: "dead-beef",
	}
	info.SetAPIEndpoint(expectEndpoint)

	err := info.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.Initialized(), jc.IsTrue)

	// Check we can read the information back
	info, err = store.ReadInfo("someenv")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.APICredentials(), gc.DeepEquals, expectCreds)
	c.Assert(info.APIEndpoint(), gc.DeepEquals, expectEndpoint)

	// Change the information and write it again.
	expectCreds.User = "arble"
	info.SetAPICredentials(expectCreds)
	err = info.Write()
	c.Assert(err, jc.ErrorIsNil)

	// Check we can read the information back
	info, err = store.ReadInfo("someenv")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.APICredentials(), gc.DeepEquals, expectCreds)
}

func (s *interfaceSuite) TestWriteTwice(c *gc.C) {
	store := s.NewStore(c)

	// Create the info.
	info := store.CreateInfo("someenv")

	// Set it up with some actual data and write it out.
	expectCreds := configstore.APICredentials{
		User:     "foobie",
		Password: "bletch",
	}
	info.SetAPICredentials(expectCreds)
	err := info.Write()
	c.Assert(err, jc.ErrorIsNil)

	expectEndpoint := configstore.APIEndpoint{
		Addresses:   []string{"0.1.2.3"},
		Hostnames:   []string{"example.invalid"},
		CACert:      "a cert",
		EnvironUUID: "dead-beef",
	}
	info.SetAPIEndpoint(expectEndpoint)
	err = info.Write()
	c.Assert(err, jc.ErrorIsNil)

	// Check we can read the information back
	again, err := store.ReadInfo("someenv")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(again.APICredentials(), gc.DeepEquals, expectCreds)
	c.Assert(again.APIEndpoint(), gc.DeepEquals, expectEndpoint)
}

func (s *interfaceSuite) TestDestroy(c *gc.C) {
	store := s.NewStore(c)

	info := store.CreateInfo("someenv")
	// Destroying something that hasn't been written is fine.
	err := info.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	err = info.Write()
	c.Assert(err, jc.ErrorIsNil)

	err = info.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	err = info.Destroy()
	c.Assert(err, gc.ErrorMatches, "environment info has already been removed")
}

func (s *interfaceSuite) TestNoBleedThrough(c *gc.C) {
	store := s.NewStore(c)

	info := store.CreateInfo("someenv")

	info.SetAPICredentials(configstore.APICredentials{User: "foo"})
	info.SetAPIEndpoint(configstore.APIEndpoint{CACert: "blah"})
	attrs := map[string]interface{}{"foo": "bar"}
	info.SetBootstrapConfig(attrs)

	err := info.Write()
	c.Assert(err, jc.ErrorIsNil)

	attrs["foo"] = "different"

	info1, err := store.ReadInfo("someenv")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info1.Initialized(), jc.IsTrue)
	c.Assert(info1.BootstrapConfig(), gc.DeepEquals, map[string]interface{}{"foo": "bar"})
}

func (s *interfaceSuite) TestSetBootstrapConfigPanicsWhenNotCreated(c *gc.C) {
	store := s.NewStore(c)

	info := store.CreateInfo("someenv")
	info.SetBootstrapConfig(map[string]interface{}{"foo": "bar"})
	err := info.Write()
	c.Assert(err, jc.ErrorIsNil)

	info, err = store.ReadInfo("someenv")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(func() { info.SetBootstrapConfig(nil) }, gc.PanicMatches, "bootstrap config set on environment info that has not just been created")
}
