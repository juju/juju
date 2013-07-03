// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	"io/ioutil"

	gc "launchpad.net/gocheck"
	"launchpad.net/goyaml"

	"launchpad.net/juju-core/environs"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	jc "launchpad.net/juju-core/testing/checkers"
)

type StateSuite struct{}

var _ = gc.Suite(&StateSuite{})

// makeDummyStorage creates a local storage.
// Returns a cleanup function that must be called when done with the storage.
func makeDummyStorage(c *gc.C) (environs.Storage, func()) {
	listener, storage, _ := envtesting.CreateLocalTestStorage(c)
	cleanup := func() { listener.Close() }
	return storage, cleanup
}

func (suite *StateSuite) TestSaveProviderStateWritesStateFile(c *gc.C) {
	storage, cleanup := makeDummyStorage(c)
	defer cleanup()
	instId := instance.Id("an-instance-id")
	state := environs.BootstrapState{StateInstances: []instance.Id{instId}}
	marshaledState, err := goyaml.Marshal(state)
	c.Assert(err, gc.IsNil)

	err = environs.SaveProviderState(storage, instId)
	c.Assert(err, gc.IsNil)

	loadedState, err := storage.Get(environs.StateFile)
	c.Assert(err, gc.IsNil)
	content, err := ioutil.ReadAll(loadedState)
	c.Assert(err, gc.IsNil)
	c.Check(content, gc.DeepEquals, marshaledState)
}

func (suite *StateSuite) TestLoadProviderStateReturnsNotFoundErrorForMissingFile(c *gc.C) {
	storage, cleanup := makeDummyStorage(c)
	defer cleanup()

	_, err := environs.LoadProviderState(storage)
	c.Check(err, jc.Satisfies, errors.IsNotFoundError)
}

func (suite *StateSuite) TestLoadStateIntegratesWithSaveProviderState(c *gc.C) {
	storage, cleanup := makeDummyStorage(c)
	defer cleanup()

	instances := []instance.Id{"un-instant-s'il-vous-plait", "id-987654", "machine-26-lxc-4"}
	err := environs.SaveProviderState(storage, instances...)
	c.Assert(err, gc.IsNil)

	storedState, err := environs.LoadProviderState(storage)
	c.Assert(err, gc.IsNil)
	c.Check(storedState, gc.DeepEquals, instances)
}
