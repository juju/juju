// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"io"
	"os"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/storage"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/provider/common"
	coretesting "github.com/juju/juju/internal/testing"
)

type StateSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&StateSuite{})

func (suite *StateSuite) newStorageWithDataDir(c *gc.C) (storage.Storage, string) {
	closer, stor, dataDir := envtesting.CreateLocalTestStorage(c)
	suite.AddCleanup(func(*gc.C) { closer.Close() })
	envtesting.UploadFakeTools(c, stor, "released")
	return stor, dataDir
}

func (suite *StateSuite) newStorage(c *gc.C) storage.Storage {
	stor, _ := suite.newStorageWithDataDir(c)
	return stor
}

func (suite *StateSuite) TestCreateStateFileWritesEmptyStateFile(c *gc.C) {
	stor := suite.newStorage(c)

	url, err := common.CreateStateFile(stor)
	c.Assert(err, jc.ErrorIsNil)

	reader, err := storage.Get(stor, common.StateFile)
	c.Assert(err, jc.ErrorIsNil)
	data, err := io.ReadAll(reader)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(data), gc.Equals, "")
	c.Assert(url, gc.NotNil)
	expectedURL, err := stor.URL(common.StateFile)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(url, gc.Equals, expectedURL)
}

func (suite *StateSuite) TestDeleteStateFile(c *gc.C) {
	closer, stor, dataDir := envtesting.CreateLocalTestStorage(c)
	defer closer.Close()

	err := common.DeleteStateFile(stor)
	c.Assert(err, jc.ErrorIsNil) // doesn't exist, juju don't care

	_, err = common.CreateStateFile(stor)
	c.Assert(err, jc.ErrorIsNil)
	_, err = os.Stat(filepath.Join(dataDir, common.StateFile))
	c.Assert(err, jc.ErrorIsNil)

	err = common.DeleteStateFile(stor)
	c.Assert(err, jc.ErrorIsNil)
	_, err = os.Stat(filepath.Join(dataDir, common.StateFile))
	c.Assert(err, jc.Satisfies, os.IsNotExist)
}

func (suite *StateSuite) TestSaveStateWritesStateFile(c *gc.C) {
	stor := suite.newStorage(c)
	state := common.BootstrapState{
		StateInstances: []instance.Id{instance.Id("an-instance-id")},
	}
	marshaledState, err := goyaml.Marshal(state)
	c.Assert(err, jc.ErrorIsNil)

	err = common.SaveState(stor, &state)
	c.Assert(err, jc.ErrorIsNil)

	loadedState, err := storage.Get(stor, common.StateFile)
	c.Assert(err, jc.ErrorIsNil)
	content, err := io.ReadAll(loadedState)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(content, gc.DeepEquals, marshaledState)
}

func (suite *StateSuite) setUpSavedState(c *gc.C, dataDir string) common.BootstrapState {
	state := common.BootstrapState{
		StateInstances: []instance.Id{instance.Id("an-instance-id")},
	}
	content, err := goyaml.Marshal(state)
	c.Assert(err, jc.ErrorIsNil)
	err = os.WriteFile(filepath.Join(dataDir, common.StateFile), content, 0644)
	c.Assert(err, jc.ErrorIsNil)
	return state
}

func (suite *StateSuite) TestLoadStateReadsStateFile(c *gc.C) {
	storage, dataDir := suite.newStorageWithDataDir(c)
	state := suite.setUpSavedState(c, dataDir)
	storedState, err := common.LoadState(storage)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(*storedState, gc.DeepEquals, state)
}

func (suite *StateSuite) TestLoadStateMissingFile(c *gc.C) {
	stor := suite.newStorage(c)
	_, err := common.LoadState(stor)
	c.Check(err, gc.Equals, environs.ErrNotBootstrapped)
}

func (suite *StateSuite) TestLoadStateIntegratesWithSaveState(c *gc.C) {
	storage := suite.newStorage(c)
	state := common.BootstrapState{
		StateInstances: []instance.Id{instance.Id("an-instance-id")},
	}
	err := common.SaveState(storage, &state)
	c.Assert(err, jc.ErrorIsNil)
	storedState, err := common.LoadState(storage)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(*storedState, gc.DeepEquals, state)
}

func (suite *StateSuite) TestAddStateInstance(c *gc.C) {
	storage := suite.newStorage(c)
	for _, str := range []string{"a", "b", "c"} {
		id := instance.Id(str)
		err := common.AddStateInstance(storage, id)
		c.Assert(err, jc.ErrorIsNil)
	}

	storedState, err := common.LoadState(storage)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(storedState, gc.DeepEquals, &common.BootstrapState{
		StateInstances: []instance.Id{
			instance.Id("a"),
			instance.Id("b"),
			instance.Id("c"),
		},
	})
}

func (suite *StateSuite) TestRemoveStateInstancesPartial(c *gc.C) {
	storage := suite.newStorage(c)
	state := common.BootstrapState{
		StateInstances: []instance.Id{
			instance.Id("a"),
			instance.Id("b"),
			instance.Id("c"),
		},
	}
	err := common.SaveState(storage, &state)
	c.Assert(err, jc.ErrorIsNil)

	err = common.RemoveStateInstances(
		storage,
		state.StateInstances[0],
		instance.Id("not-there"),
		state.StateInstances[2],
	)
	c.Assert(err, jc.ErrorIsNil)

	storedState, err := common.LoadState(storage)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storedState, gc.DeepEquals, &common.BootstrapState{
		StateInstances: []instance.Id{
			state.StateInstances[1],
		},
	})
}

func (suite *StateSuite) TestRemoveStateInstancesNone(c *gc.C) {
	storage := suite.newStorage(c)
	state := common.BootstrapState{
		StateInstances: []instance.Id{instance.Id("an-instance-id")},
	}
	err := common.SaveState(storage, &state)
	c.Assert(err, jc.ErrorIsNil)

	err = common.RemoveStateInstances(
		storage,
		instance.Id("not-there"),
	)
	c.Assert(err, jc.ErrorIsNil)

	storedState, err := common.LoadState(storage)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storedState, gc.DeepEquals, &state)
}

func (suite *StateSuite) TestRemoveStateInstancesNoProviderState(c *gc.C) {
	storage := suite.newStorage(c)
	err := common.RemoveStateInstances(storage, instance.Id("id"))
	// No error if the id is missing, so no error if the entire
	// provider-state file is missing. This is the case if
	// bootstrap failed.
	c.Assert(err, jc.ErrorIsNil)
}
