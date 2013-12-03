// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap_test

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"

	gc "launchpad.net/gocheck"
	"launchpad.net/goyaml"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/bootstrap"
	"launchpad.net/juju-core/environs/storage"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/instance"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
)

type StateSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&StateSuite{})

func (suite *StateSuite) newStorage(c *gc.C) storage.Storage {
	closer, stor, _ := envtesting.CreateLocalTestStorage(c)
	suite.AddCleanup(func(*gc.C) { closer.Close() })
	envtesting.UploadFakeTools(c, stor)
	return stor
}

func (suite *StateSuite) TestCreateStateFileWritesEmptyStateFile(c *gc.C) {
	stor := suite.newStorage(c)

	url, err := bootstrap.CreateStateFile(stor)
	c.Assert(err, gc.IsNil)

	reader, err := storage.Get(stor, bootstrap.StateFile)
	c.Assert(err, gc.IsNil)
	data, err := ioutil.ReadAll(reader)
	c.Assert(err, gc.IsNil)
	c.Check(string(data), gc.Equals, "")
	c.Assert(url, gc.NotNil)
	expectedURL, err := stor.URL(bootstrap.StateFile)
	c.Assert(err, gc.IsNil)
	c.Check(url, gc.Equals, expectedURL)
}

func (suite *StateSuite) TestDeleteStateFile(c *gc.C) {
	closer, stor, dataDir := envtesting.CreateLocalTestStorage(c)
	defer closer.Close()

	err := bootstrap.DeleteStateFile(stor)
	c.Assert(err, gc.IsNil) // doesn't exist, juju don't care

	_, err = bootstrap.CreateStateFile(stor)
	c.Assert(err, gc.IsNil)
	_, err = os.Stat(filepath.Join(dataDir, bootstrap.StateFile))
	c.Assert(err, gc.IsNil)

	err = bootstrap.DeleteStateFile(stor)
	c.Assert(err, gc.IsNil)
	_, err = os.Stat(filepath.Join(dataDir, bootstrap.StateFile))
	c.Assert(err, jc.Satisfies, os.IsNotExist)
}

func (suite *StateSuite) TestSaveStateWritesStateFile(c *gc.C) {
	stor := suite.newStorage(c)
	arch := "amd64"
	state := bootstrap.BootstrapState{
		StateInstances:  []instance.Id{instance.Id("an-instance-id")},
		Characteristics: []instance.HardwareCharacteristics{{Arch: &arch}}}
	marshaledState, err := goyaml.Marshal(state)
	c.Assert(err, gc.IsNil)

	err = bootstrap.SaveState(stor, &state)
	c.Assert(err, gc.IsNil)

	loadedState, err := storage.Get(stor, bootstrap.StateFile)
	c.Assert(err, gc.IsNil)
	content, err := ioutil.ReadAll(loadedState)
	c.Assert(err, gc.IsNil)
	c.Check(content, gc.DeepEquals, marshaledState)
}

func (suite *StateSuite) setUpSavedState(c *gc.C, stor storage.Storage) bootstrap.BootstrapState {
	arch := "amd64"
	state := bootstrap.BootstrapState{
		StateInstances:  []instance.Id{instance.Id("an-instance-id")},
		Characteristics: []instance.HardwareCharacteristics{{Arch: &arch}}}
	content, err := goyaml.Marshal(state)
	c.Assert(err, gc.IsNil)
	err = stor.Put(bootstrap.StateFile, ioutil.NopCloser(bytes.NewReader(content)), int64(len(content)))
	c.Assert(err, gc.IsNil)
	return state
}

func (suite *StateSuite) TestLoadStateReadsStateFile(c *gc.C) {
	storage := suite.newStorage(c)
	state := suite.setUpSavedState(c, storage)
	storedState, err := bootstrap.LoadState(storage)
	c.Assert(err, gc.IsNil)
	c.Check(*storedState, gc.DeepEquals, state)
}

func (suite *StateSuite) TestLoadStateFromURLReadsStateFile(c *gc.C) {
	stor := suite.newStorage(c)
	state := suite.setUpSavedState(c, stor)
	url, err := stor.URL(bootstrap.StateFile)
	c.Assert(err, gc.IsNil)
	storedState, err := bootstrap.LoadStateFromURL(url)
	c.Assert(err, gc.IsNil)
	c.Check(*storedState, gc.DeepEquals, state)
}

func (suite *StateSuite) TestLoadStateMissingFile(c *gc.C) {
	stor := suite.newStorage(c)
	_, err := bootstrap.LoadState(stor)
	c.Check(err, gc.Equals, environs.ErrNotBootstrapped)
}

func (suite *StateSuite) TestLoadStateIntegratesWithSaveState(c *gc.C) {
	storage := suite.newStorage(c)
	arch := "amd64"
	state := bootstrap.BootstrapState{
		StateInstances:  []instance.Id{instance.Id("an-instance-id")},
		Characteristics: []instance.HardwareCharacteristics{{Arch: &arch}}}
	err := bootstrap.SaveState(storage, &state)
	c.Assert(err, gc.IsNil)
	storedState, err := bootstrap.LoadState(storage)
	c.Assert(err, gc.IsNil)

	c.Check(*storedState, gc.DeepEquals, state)
}
