// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/juju/tc"
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

func TestStateSuite(t *testing.T) {
	tc.Run(t, &StateSuite{})
}

func (suite *StateSuite) newStorageWithDataDir(c *tc.C) (storage.Storage, string) {
	closer, stor, dataDir := envtesting.CreateLocalTestStorage(c)
	suite.AddCleanup(func(*tc.C) { closer.Close() })
	envtesting.UploadFakeTools(c, stor, "released")
	return stor, dataDir
}

func (suite *StateSuite) newStorage(c *tc.C) storage.Storage {
	stor, _ := suite.newStorageWithDataDir(c)
	return stor
}

func (suite *StateSuite) TestCreateStateFileWritesEmptyStateFile(c *tc.C) {
	stor := suite.newStorage(c)

	url, err := common.CreateStateFile(stor)
	c.Assert(err, tc.ErrorIsNil)

	reader, err := storage.Get(stor, common.StateFile)
	c.Assert(err, tc.ErrorIsNil)
	data, err := io.ReadAll(reader)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(data), tc.Equals, "")
	c.Assert(url, tc.NotNil)
	expectedURL, err := stor.URL(common.StateFile)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(url, tc.Equals, expectedURL)
}

func (suite *StateSuite) TestDeleteStateFile(c *tc.C) {
	closer, stor, dataDir := envtesting.CreateLocalTestStorage(c)
	defer closer.Close()

	err := common.DeleteStateFile(stor)
	c.Assert(err, tc.ErrorIsNil) // doesn't exist, juju don't care

	_, err = common.CreateStateFile(stor)
	c.Assert(err, tc.ErrorIsNil)
	_, err = os.Stat(filepath.Join(dataDir, common.StateFile))
	c.Assert(err, tc.ErrorIsNil)

	err = common.DeleteStateFile(stor)
	c.Assert(err, tc.ErrorIsNil)
	_, err = os.Stat(filepath.Join(dataDir, common.StateFile))
	c.Assert(err, tc.Satisfies, os.IsNotExist)
}

func (suite *StateSuite) TestSaveStateWritesStateFile(c *tc.C) {
	stor := suite.newStorage(c)
	state := common.BootstrapState{
		StateInstances: []instance.Id{instance.Id("an-instance-id")},
	}
	marshaledState, err := goyaml.Marshal(state)
	c.Assert(err, tc.ErrorIsNil)

	err = common.SaveState(stor, &state)
	c.Assert(err, tc.ErrorIsNil)

	loadedState, err := storage.Get(stor, common.StateFile)
	c.Assert(err, tc.ErrorIsNil)
	content, err := io.ReadAll(loadedState)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(content, tc.DeepEquals, marshaledState)
}

func (suite *StateSuite) setUpSavedState(c *tc.C, dataDir string) common.BootstrapState {
	state := common.BootstrapState{
		StateInstances: []instance.Id{instance.Id("an-instance-id")},
	}
	content, err := goyaml.Marshal(state)
	c.Assert(err, tc.ErrorIsNil)
	err = os.WriteFile(filepath.Join(dataDir, common.StateFile), content, 0644)
	c.Assert(err, tc.ErrorIsNil)
	return state
}

func (suite *StateSuite) TestLoadStateReadsStateFile(c *tc.C) {
	storage, dataDir := suite.newStorageWithDataDir(c)
	state := suite.setUpSavedState(c, dataDir)
	storedState, err := common.LoadState(storage)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(*storedState, tc.DeepEquals, state)
}

func (suite *StateSuite) TestLoadStateMissingFile(c *tc.C) {
	stor := suite.newStorage(c)
	_, err := common.LoadState(stor)
	c.Check(err, tc.Equals, environs.ErrNotBootstrapped)
}

func (suite *StateSuite) TestLoadStateIntegratesWithSaveState(c *tc.C) {
	storage := suite.newStorage(c)
	state := common.BootstrapState{
		StateInstances: []instance.Id{instance.Id("an-instance-id")},
	}
	err := common.SaveState(storage, &state)
	c.Assert(err, tc.ErrorIsNil)
	storedState, err := common.LoadState(storage)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(*storedState, tc.DeepEquals, state)
}

func (suite *StateSuite) TestAddStateInstance(c *tc.C) {
	storage := suite.newStorage(c)
	for _, str := range []string{"a", "b", "c"} {
		id := instance.Id(str)
		err := common.AddStateInstance(storage, id)
		c.Assert(err, tc.ErrorIsNil)
	}

	storedState, err := common.LoadState(storage)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(storedState, tc.DeepEquals, &common.BootstrapState{
		StateInstances: []instance.Id{
			instance.Id("a"),
			instance.Id("b"),
			instance.Id("c"),
		},
	})
}

func (suite *StateSuite) TestRemoveStateInstancesPartial(c *tc.C) {
	storage := suite.newStorage(c)
	state := common.BootstrapState{
		StateInstances: []instance.Id{
			instance.Id("a"),
			instance.Id("b"),
			instance.Id("c"),
		},
	}
	err := common.SaveState(storage, &state)
	c.Assert(err, tc.ErrorIsNil)

	err = common.RemoveStateInstances(
		storage,
		state.StateInstances[0],
		instance.Id("not-there"),
		state.StateInstances[2],
	)
	c.Assert(err, tc.ErrorIsNil)

	storedState, err := common.LoadState(storage)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(storedState, tc.DeepEquals, &common.BootstrapState{
		StateInstances: []instance.Id{
			state.StateInstances[1],
		},
	})
}

func (suite *StateSuite) TestRemoveStateInstancesNone(c *tc.C) {
	storage := suite.newStorage(c)
	state := common.BootstrapState{
		StateInstances: []instance.Id{instance.Id("an-instance-id")},
	}
	err := common.SaveState(storage, &state)
	c.Assert(err, tc.ErrorIsNil)

	err = common.RemoveStateInstances(
		storage,
		instance.Id("not-there"),
	)
	c.Assert(err, tc.ErrorIsNil)

	storedState, err := common.LoadState(storage)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(storedState, tc.DeepEquals, &state)
}

func (suite *StateSuite) TestRemoveStateInstancesNoProviderState(c *tc.C) {
	storage := suite.newStorage(c)
	err := common.RemoveStateInstances(storage, instance.Id("id"))
	// No error if the id is missing, so no error if the entire
	// provider-state file is missing. This is the case if
	// bootstrap failed.
	c.Assert(err, tc.ErrorIsNil)
}
