// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	"bytes"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/localstorage"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
)

type StateSuite struct{}

var _ = Suite(&StateSuite{})

// makeDummyStorage creates a local storage.
// Returns a cleanup function that must be called when done with the storage.
func makeDummyStorage(c *C) (environs.Storage, func()) {
	listener, err := localstorage.Serve("127.0.0.1:0", c.MkDir())
	c.Assert(err, IsNil)
	storage := localstorage.Client(listener.Addr().String())
	cleanup := func() { listener.Close() }
	return storage, cleanup
}

func (suite *StateSuite) TestSaveStateWritesStateFile(c *C) {
	storage, cleanup := makeDummyStorage(c)
	defer cleanup()
	state := environs.BootstrapState{StateInstances: []instance.Id{"an-instance-id"}}
	marshaledState, err := goyaml.Marshal(state)
	c.Assert(err, IsNil)

	err = environs.SaveState(storage, &state)
	c.Assert(err, IsNil)

	loadedState, err := storage.Get(environs.StateFile)
	c.Assert(err, IsNil)
	content, err := ioutil.ReadAll(loadedState)
	c.Assert(err, IsNil)
	c.Check(content, DeepEquals, marshaledState)
}

func (suite *StateSuite) TestLoadStateReadsStateFile(c *C) {
	storage, cleanup := makeDummyStorage(c)
	defer cleanup()
	state := environs.BootstrapState{StateInstances: []instance.Id{"id-goes-here"}}
	content, err := goyaml.Marshal(state)
	c.Assert(err, IsNil)
	err = storage.Put(environs.StateFile, ioutil.NopCloser(bytes.NewReader(content)), int64(len(content)))
	c.Assert(err, IsNil)

	storedState, err := environs.LoadState(storage)
	c.Assert(err, IsNil)

	c.Check(*storedState, DeepEquals, state)
}

func (suite *StateSuite) TestLoadStateReturnsNotFoundErrorForMissingFile(c *C) {
	storage, cleanup := makeDummyStorage(c)
	defer cleanup()

	_, err := environs.LoadState(storage)

	c.Check(errors.IsNotFoundError(err), Equals, true)
}

func (suite *StateSuite) TestLoadStateIntegratesWithSaveState(c *C) {
	storage, cleanup := makeDummyStorage(c)
	defer cleanup()
	state := environs.BootstrapState{StateInstances: []instance.Id{"un-instant-s'il-vous-plait"}}

	err := environs.SaveState(storage, &state)
	c.Assert(err, IsNil)
	storedState, err := environs.LoadState(storage)
	c.Assert(err, IsNil)

	c.Check(*storedState, DeepEquals, state)
}
