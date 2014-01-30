// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"path/filepath"

	gc "launchpad.net/gocheck"
	"launchpad.net/goyaml"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/storage"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/provider/common"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/testbase"
)

type StateSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&StateSuite{})

type cleaner interface {
	AddCleanup(testbase.CleanupFunc)
}

func newStorageWithDataDir(suite cleaner, c *gc.C) (stor storage.Storage, dataDir string) {
	closer, stor, dataDir := envtesting.CreateLocalTestStorage(c)
	suite.AddCleanup(func(*gc.C) { closer.Close() })
	return
}

func newStorage(suite cleaner, c *gc.C) (stor storage.Storage) {
	stor, _ = newStorageWithDataDir(suite, c)
	return
}

// testingHTTPServer creates a tempdir backed https server with invalid certs
func testingHTTPSServer(suite cleaner, c *gc.C) (baseURL string, dataDir string) {
	dataDir = c.MkDir()
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir(dataDir)))
	server := httptest.NewTLSServer(mux)
	suite.AddCleanup(func(*gc.C) { server.Close() })
	baseURL = server.URL
	return
}

func (suite *StateSuite) TestCreateStateFileWritesEmptyStateFile(c *gc.C) {
	stor := newStorage(suite, c)

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

func (suite *StateSuite) TestSaveStateWritesStateFile(c *gc.C) {
	stor := newStorage(suite, c)
	arch := "amd64"
	state := common.BootstrapState{
		StateInstances:  []instance.Id{instance.Id("an-instance-id")},
		Characteristics: []instance.HardwareCharacteristics{{Arch: &arch}}}
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
	arch := "amd64"
	state := common.BootstrapState{
		StateInstances:  []instance.Id{instance.Id("an-instance-id")},
		Characteristics: []instance.HardwareCharacteristics{{Arch: &arch}}}
	content, err := goyaml.Marshal(state)
	c.Assert(err, gc.IsNil)
	err = ioutil.WriteFile(filepath.Join(dataDir, common.StateFile), []byte(content), 0644)
	c.Assert(err, gc.IsNil)
	return state
}

func (suite *StateSuite) TestLoadStateReadsStateFile(c *gc.C) {
	storage, dataDir := newStorageWithDataDir(suite, c)
	state := suite.setUpSavedState(c, dataDir)
	storedState, err := common.LoadState(storage)
	c.Assert(err, gc.IsNil)
	c.Check(*storedState, gc.DeepEquals, state)
}

func (suite *StateSuite) TestLoadStateFromURLReadsStateFile(c *gc.C) {
	stor, dataDir := newStorageWithDataDir(suite, c)
	state := suite.setUpSavedState(c, dataDir)
	url, err := stor.URL(common.StateFile)
	c.Assert(err, gc.IsNil)
	storedState, err := common.LoadStateFromURL(url, false)
	c.Assert(err, gc.IsNil)
	c.Check(*storedState, gc.DeepEquals, state)
}

func (suite *StateSuite) TestLoadStateFromURLBadCert(c *gc.C) {
	baseURL, _ := testingHTTPSServer(suite, c)
	url := baseURL + "/" + common.StateFile
	storedState, err := common.LoadStateFromURL(url, false)
	c.Assert(err, gc.ErrorMatches, ".*/provider-state:.* certificate signed by unknown authority")
	c.Assert(storedState, gc.IsNil)
}

func (suite *StateSuite) TestLoadStateFromURLBadCertPermitted(c *gc.C) {
	baseURL, dataDir := testingHTTPSServer(suite, c)
	state := suite.setUpSavedState(c, dataDir)
	url := baseURL + "/" + common.StateFile
	storedState, err := common.LoadStateFromURL(url, true)
	c.Assert(err, gc.IsNil)
	c.Check(*storedState, gc.DeepEquals, state)
}

func (suite *StateSuite) TestLoadStateMissingFile(c *gc.C) {
	stor := newStorage(suite, c)
	_, err := common.LoadState(stor)
	c.Check(err, gc.Equals, environs.ErrNotBootstrapped)
}

func (suite *StateSuite) TestLoadStateIntegratesWithSaveState(c *gc.C) {
	storage := newStorage(suite, c)
	arch := "amd64"
	state := common.BootstrapState{
		StateInstances:  []instance.Id{instance.Id("an-instance-id")},
		Characteristics: []instance.HardwareCharacteristics{{Arch: &arch}}}
	err := common.SaveState(storage, &state)
	c.Assert(err, gc.IsNil)
	storedState, err := common.LoadState(storage)
	c.Assert(err, gc.IsNil)

	c.Check(*storedState, gc.DeepEquals, state)
}

func (suite *StateSuite) TestGetDNSNamesAcceptsNil(c *gc.C) {
	result := common.GetDNSNames(nil)
	c.Check(result, gc.DeepEquals, []string{})
}

func (suite *StateSuite) TestGetDNSNamesReturnsNames(c *gc.C) {
	instances := []instance.Instance{
		&dnsNameFakeInstance{name: "foo"},
		&dnsNameFakeInstance{name: "bar"},
	}

	c.Check(common.GetDNSNames(instances), gc.DeepEquals, []string{"foo", "bar"})
}

func (suite *StateSuite) TestGetDNSNamesIgnoresNils(c *gc.C) {
	c.Check(common.GetDNSNames([]instance.Instance{nil, nil}), gc.DeepEquals, []string{})
}

func (suite *StateSuite) TestGetDNSNamesIgnoresInstancesWithoutNames(c *gc.C) {
	instances := []instance.Instance{&dnsNameFakeInstance{err: instance.ErrNoDNSName}}
	c.Check(common.GetDNSNames(instances), gc.DeepEquals, []string{})
}

func (suite *StateSuite) TestGetDNSNamesIgnoresInstancesWithBlankNames(c *gc.C) {
	instances := []instance.Instance{&dnsNameFakeInstance{name: ""}}
	c.Check(common.GetDNSNames(instances), gc.DeepEquals, []string{})
}

func (suite *StateSuite) TestComposeAddressesAcceptsNil(c *gc.C) {
	c.Check(common.ComposeAddresses(nil, 1433), gc.DeepEquals, []string{})
}

func (suite *StateSuite) TestComposeAddressesSuffixesAddresses(c *gc.C) {
	c.Check(
		common.ComposeAddresses([]string{"onehost", "otherhost"}, 1957),
		gc.DeepEquals,
		[]string{"onehost:1957", "otherhost:1957"})
}

func (suite *StateSuite) TestGetStateInfo(c *gc.C) {
	cert := testing.CACert
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"ca-cert":    cert,
		"state-port": 123,
		"api-port":   456,
	})
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	hostnames := []string{"onehost", "otherhost"}

	stateInfo, apiInfo := common.GetStateInfo(cfg, hostnames)

	c.Check(stateInfo.Addrs, gc.DeepEquals, []string{"onehost:123", "otherhost:123"})
	c.Check(string(stateInfo.CACert), gc.Equals, cert)
	c.Check(apiInfo.Addrs, gc.DeepEquals, []string{"onehost:456", "otherhost:456"})
	c.Check(string(apiInfo.CACert), gc.Equals, cert)
}
