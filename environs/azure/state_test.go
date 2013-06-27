// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"bytes"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/environs/local"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"net"
	"sync"
)

type StateSuite struct {
	ProviderSuite
}

var _ = Suite(&StateSuite{})

var listenerMu sync.Mutex

// setDummyStorage injects the local provider's fake storage implementation
// into the given environment, so that tests can manipulate storage as if it
// were real.
// Returns a cleanup function that must be called when done with the storage.
func setDummyStorage(c *C, env *azureEnviron) func() {
	listenerMu.Lock()
	defer listenerMu.Unlock()

	dataDir := c.MkDir()
	listener, err := local.Listen(dataDir, "test-environ", "127.0.0.1", 0)
	c.Assert(err, IsNil)
	port := listener.Addr().(*net.TCPAddr).Port
	env.storage = local.NewStorage("127.0.0.1", port)
	return func() { listener.Close() }
}

func (suite *StateSuite) TestSaveStateWritesStateFile(c *C) {
	env := makeEnviron(c)
	cleanup := setDummyStorage(c, env)
	defer cleanup()
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
	env := makeEnviron(c)
	cleanup := setDummyStorage(c, env)
	defer cleanup()
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
	env := makeEnviron(c)
	cleanup := setDummyStorage(c, env)
	defer cleanup()

	_, err := env.loadState()

	c.Check(errors.IsNotFoundError(err), Equals, true)
}

func (suite *StateSuite) TestLoadStateIntegratesWithSaveState(c *C) {
	env := makeEnviron(c)
	cleanup := setDummyStorage(c, env)
	defer cleanup()
	state := bootstrapState{StateInstances: []instance.Id{"un-instant-s'il-vous-plait"}}

	err := env.saveState(&state)
	c.Assert(err, IsNil)
	storedState, err := env.loadState()
	c.Assert(err, IsNil)

	c.Check(*storedState, DeepEquals, state)
}
