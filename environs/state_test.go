// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	"bytes"
	"io/ioutil"

	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/localstorage"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/checkers"
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

func (*StateSuite) TestCreateStateFileWritesEmptyStateFile(c *C) {
	storage, cleanup := makeDummyStorage(c)
	defer cleanup()

	url, err := environs.CreateStateFile(storage)
	c.Assert(err, IsNil)

	reader, err := storage.Get(environs.StateFile)
	c.Assert(err, IsNil)
	data, err := ioutil.ReadAll(reader)
	c.Assert(err, IsNil)
	c.Check(string(data), Equals, "")
	c.Assert(url, NotNil)
	expectedURL, err := storage.URL(environs.StateFile)
	c.Assert(err, IsNil)
	c.Check(url, Equals, expectedURL)
}

func (suite *StateSuite) TestSaveStateWritesStateFile(c *C) {
	storage, cleanup := makeDummyStorage(c)
	defer cleanup()
	arch := "amd64"
	state := environs.BootstrapState{
		StateInstances:  []instance.Id{instance.Id("an-instance-id")},
		Characteristics: []instance.HardwareCharacteristics{{Arch: &arch}}}
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

func (suite *StateSuite) setUpSavedState(c *C, storage environs.Storage) environs.BootstrapState {
	arch := "amd64"
	state := environs.BootstrapState{
		StateInstances:  []instance.Id{instance.Id("an-instance-id")},
		Characteristics: []instance.HardwareCharacteristics{{Arch: &arch}}}
	content, err := goyaml.Marshal(state)
	c.Assert(err, IsNil)
	err = storage.Put(environs.StateFile, ioutil.NopCloser(bytes.NewReader(content)), int64(len(content)))
	c.Assert(err, IsNil)
	return state
}

func (suite *StateSuite) TestLoadStateReadsStateFile(c *C) {
	storage, cleanup := makeDummyStorage(c)
	defer cleanup()
	state := suite.setUpSavedState(c, storage)
	storedState, err := environs.LoadState(storage)
	c.Assert(err, IsNil)
	c.Check(*storedState, DeepEquals, state)
}

func (suite *StateSuite) TestLoadStateFromURLReadsStateFile(c *C) {
	storage, cleanup := makeDummyStorage(c)
	defer cleanup()
	state := suite.setUpSavedState(c, storage)
	url, err := storage.URL(environs.StateFile)
	c.Assert(err, IsNil)
	storedState, err := environs.LoadStateFromURL(url)
	c.Assert(err, IsNil)
	c.Check(*storedState, DeepEquals, state)
}

func (suite *StateSuite) TestLoadStateMissingFile(c *C) {
	storage, cleanup := makeDummyStorage(c)
	defer cleanup()

	_, err := environs.LoadState(storage)

	c.Check(err, checkers.Satisfies, errors.IsNotBootstrapped)
}

func (suite *StateSuite) TestLoadStateIntegratesWithSaveState(c *C) {
	storage, cleanup := makeDummyStorage(c)
	defer cleanup()
	arch := "amd64"
	state := environs.BootstrapState{
		StateInstances:  []instance.Id{instance.Id("an-instance-id")},
		Characteristics: []instance.HardwareCharacteristics{{Arch: &arch}}}
	err := environs.SaveState(storage, &state)
	c.Assert(err, IsNil)
	storedState, err := environs.LoadState(storage)
	c.Assert(err, IsNil)

	c.Check(*storedState, DeepEquals, state)
}

func (suite *StateSuite) TestGetDNSNamesAcceptsNil(c *C) {
	result := environs.GetDNSNames(nil)
	c.Check(result, DeepEquals, []string{})
}

func (suite *StateSuite) TestGetDNSNamesReturnsNames(c *C) {
	instances := []instance.Instance{
		&dnsNameFakeInstance{name: "foo"},
		&dnsNameFakeInstance{name: "bar"},
	}

	c.Check(environs.GetDNSNames(instances), DeepEquals, []string{"foo", "bar"})
}

func (suite *StateSuite) TestGetDNSNamesIgnoresNils(c *C) {
	c.Check(environs.GetDNSNames([]instance.Instance{nil, nil}), DeepEquals, []string{})
}

func (suite *StateSuite) TestGetDNSNamesIgnoresInstancesWithoutNames(c *C) {
	instances := []instance.Instance{&dnsNameFakeInstance{err: instance.ErrNoDNSName}}
	c.Check(environs.GetDNSNames(instances), DeepEquals, []string{})
}

func (suite *StateSuite) TestGetDNSNamesIgnoresInstancesWithBlankNames(c *C) {
	instances := []instance.Instance{&dnsNameFakeInstance{name: ""}}
	c.Check(environs.GetDNSNames(instances), DeepEquals, []string{})
}

func (suite *StateSuite) TestComposeAddressesAcceptsNil(c *C) {
	c.Check(environs.ComposeAddresses(nil, 1433), DeepEquals, []string{})
}

func (suite *StateSuite) TestComposeAddressesSuffixesAddresses(c *C) {
	c.Check(
		environs.ComposeAddresses([]string{"onehost", "otherhost"}, 1957),
		DeepEquals,
		[]string{"onehost:1957", "otherhost:1957"})
}

func (suite *StateSuite) TestGetStateInfo(c *C) {
	cert := testing.CACert
	cfg, err := config.New(map[string]interface{}{
		// Some config items we're going to test for:
		"ca-cert":    cert,
		"state-port": 123,
		"api-port":   456,
		// And some required but irrelevant items:
		"name":           "aname",
		"type":           "dummy",
		"ca-private-key": testing.CAKey,
	})
	c.Assert(err, IsNil)
	hostnames := []string{"onehost", "otherhost"}

	stateInfo, apiInfo := environs.GetStateInfo(cfg, hostnames)

	c.Check(stateInfo.Addrs, DeepEquals, []string{"onehost:123", "otherhost:123"})
	c.Check(string(stateInfo.CACert), Equals, cert)
	c.Check(apiInfo.Addrs, DeepEquals, []string{"onehost:456", "otherhost:456"})
	c.Check(string(apiInfo.CACert), Equals, cert)
}
