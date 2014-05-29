// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap_test

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"
	"launchpad.net/goyaml"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/bootstrap"
	"launchpad.net/juju-core/environs/storage"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/instance"
	coretesting "launchpad.net/juju-core/testing"
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
	state := bootstrap.BootstrapState{
		StateInstances: []instance.Id{instance.Id("an-instance-id")},
	}
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

func (suite *StateSuite) setUpSavedState(c *gc.C, dataDir string) bootstrap.BootstrapState {
	state := bootstrap.BootstrapState{
		StateInstances: []instance.Id{instance.Id("an-instance-id")},
	}
	content, err := goyaml.Marshal(state)
	c.Assert(err, gc.IsNil)
	err = ioutil.WriteFile(filepath.Join(dataDir, bootstrap.StateFile), []byte(content), 0644)
	c.Assert(err, gc.IsNil)
	return state
}

func (suite *StateSuite) TestLoadStateReadsStateFile(c *gc.C) {
	storage, dataDir := suite.newStorageWithDataDir(c)
	state := suite.setUpSavedState(c, dataDir)
	storedState, err := bootstrap.LoadState(storage)
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
	state := bootstrap.BootstrapState{
		StateInstances: []instance.Id{instance.Id("an-instance-id")},
	}
	err := bootstrap.SaveState(storage, &state)
	c.Assert(err, gc.IsNil)
	storedState, err := bootstrap.LoadState(storage)
	c.Assert(err, gc.IsNil)

	c.Check(*storedState, gc.DeepEquals, state)
}
