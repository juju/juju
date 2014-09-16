// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	goyaml "gopkg.in/yaml.v1"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/storage"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/common"
	coretesting "github.com/juju/juju/testing"
)

type StateSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&StateSuite{})

func (suite *StateSuite) newStorageWithDataDir(c *gc.C) (storage.Storage, string) {
	closer, stor, dataDir := envtesting.CreateLocalTestStorage(c)
	suite.AddCleanup(func(*gc.C) { closer.Close() })
	envtesting.UploadFakeTools(c, stor)
	return stor, dataDir
}

func (suite *StateSuite) newStorage(c *gc.C) storage.Storage {
	stor, _ := suite.newStorageWithDataDir(c)
	return stor
}

// testingHTTPSServer creates a tempdir backed https server with internal
// self-signed certs that will not be accepted as valid.
func (suite *StateSuite) testingHTTPSServer(c *gc.C) (string, string) {
	dataDir := c.MkDir()
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir(dataDir)))
	server := httptest.NewTLSServer(mux)
	suite.AddCleanup(func(*gc.C) { server.Close() })
	return server.URL, dataDir
}

func (suite *StateSuite) TestCreateStateFileWritesEmptyStateFile(c *gc.C) {
	stor := suite.newStorage(c)

	url, err := common.CreateStateFile(stor)
	c.Assert(err, gc.IsNil)

	reader, err := storage.Get(stor, common.StateFile)
	c.Assert(err, gc.IsNil)
	data, err := ioutil.ReadAll(reader)
	c.Assert(err, gc.IsNil)
	c.Check(string(data), gc.Equals, "")
	c.Assert(url, gc.NotNil)
	expectedURL, err := stor.URL(common.StateFile)
	c.Assert(err, gc.IsNil)
	c.Check(url, gc.Equals, expectedURL)
}

func (suite *StateSuite) TestDeleteStateFile(c *gc.C) {
	closer, stor, dataDir := envtesting.CreateLocalTestStorage(c)
	defer closer.Close()

	err := common.DeleteStateFile(stor)
	c.Assert(err, gc.IsNil) // doesn't exist, juju don't care

	_, err = common.CreateStateFile(stor)
	c.Assert(err, gc.IsNil)
	_, err = os.Stat(filepath.Join(dataDir, common.StateFile))
	c.Assert(err, gc.IsNil)

	err = common.DeleteStateFile(stor)
	c.Assert(err, gc.IsNil)
	_, err = os.Stat(filepath.Join(dataDir, common.StateFile))
	c.Assert(err, jc.Satisfies, os.IsNotExist)
}

func (suite *StateSuite) TestSaveStateWritesStateFile(c *gc.C) {
	stor := suite.newStorage(c)
	state := common.BootstrapState{
		StateInstances: []instance.Id{instance.Id("an-instance-id")},
	}
	marshaledState, err := goyaml.Marshal(state)
	c.Assert(err, gc.IsNil)

	err = common.SaveState(stor, &state)
	c.Assert(err, gc.IsNil)

	loadedState, err := storage.Get(stor, common.StateFile)
	c.Assert(err, gc.IsNil)
	content, err := ioutil.ReadAll(loadedState)
	c.Assert(err, gc.IsNil)
	c.Check(content, gc.DeepEquals, marshaledState)
}

func (suite *StateSuite) setUpSavedState(c *gc.C, dataDir string) common.BootstrapState {
	state := common.BootstrapState{
		StateInstances: []instance.Id{instance.Id("an-instance-id")},
	}
	content, err := goyaml.Marshal(state)
	c.Assert(err, gc.IsNil)
	err = ioutil.WriteFile(filepath.Join(dataDir, common.StateFile), []byte(content), 0644)
	c.Assert(err, gc.IsNil)
	return state
}

func (suite *StateSuite) TestLoadStateReadsStateFile(c *gc.C) {
	storage, dataDir := suite.newStorageWithDataDir(c)
	state := suite.setUpSavedState(c, dataDir)
	storedState, err := common.LoadState(storage)
	c.Assert(err, gc.IsNil)
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
	c.Assert(err, gc.IsNil)
	storedState, err := common.LoadState(storage)
	c.Assert(err, gc.IsNil)

	c.Check(*storedState, gc.DeepEquals, state)
}
