// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"bytes"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
)

type StateSuite struct {
	ProviderSuite
}

var _ = Suite(&StateSuite{})

// setDummyStorage injects the dummy provider's fake storage implementation
// into the given environment, so that tests can manipulate storage as if it
// were real.
func setDummyStorage(env *azureEnviron) {
	dummyState := dummy.NewState("test", make(chan dummy.Operation), config.FwDefault)
	env.storage = dummy.NewStorage(dummyState, "/dummy-storage")
}

// makeEnv creates an environment rigged for state-file testing.  It uses the
// dummy provider's fake storage implementation.
func (suite *StateSuite) makeEnv(c *C) *azureEnviron {
	env := makeEnviron(c)
	setDummyStorage(env)
	return env
}

func (suite *StateSuite) TestSaveStateWritesStateFile(c *C) {
	env := suite.makeEnv(c)
	state := bootstrapState{StateInstances: []instance.Id{"an-instance-id"}}
	marshaledState, err := goyaml.Marshal(state)
	c.Assert(err, IsNil)

	err = env.saveState(&state)
	c.Assert(err, IsNil)

	loadedState, err := env.storage.Get(stateFile)
	c.Assert(err, IsNil)
	content, err := ioutil.ReadAll(loadedState)
	c.Assert(err, IsNil)
	c.Check(content, DeepEquals, marshaledState)
}

func (suite *StateSuite) TestLoadStateReadsStateFile(c *C) {
	env := suite.makeEnv(c)
	state := bootstrapState{StateInstances: []instance.Id{"id-goes-here"}}
	content, err := goyaml.Marshal(state)
	c.Assert(err, IsNil)
	err = env.storage.Put(stateFile, ioutil.NopCloser(bytes.NewReader(content)), int64(len(content)))
	c.Assert(err, IsNil)

	storedState, err := env.loadState()
	c.Assert(err, IsNil)

	c.Check(*storedState, DeepEquals, state)
}

func (suite *StateSuite) TestLoadStateReturnsNotFoundErrorForMissingFile(c *C) {
	env := suite.makeEnv(c)

	_, err := env.loadState()

	c.Check(errors.IsNotFoundError(err), Equals, true)
}

func (suite *StateSuite) TestLoadStateIntegratesWithSaveState(c *C) {
	env := suite.makeEnv(c)
	state := bootstrapState{StateInstances: []instance.Id{"un-instant-s'il-vous-plait"}}

	err := env.saveState(&state)
	c.Assert(err, IsNil)
	storedState, err := env.loadState()
	c.Assert(err, IsNil)

	c.Check(*storedState, DeepEquals, state)
}
