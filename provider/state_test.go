// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"bytes"
	"io/ioutil"

	gc "launchpad.net/gocheck"
	"launchpad.net/goyaml"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/localstorage"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/provider"
	"launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
)

type StateSuite struct{}

var _ = gc.Suite(&StateSuite{})

// makeDummyStorage creates a local storage.
// Returns a cleanup function that must be called when done with the storage.
func makeDummyStorage(c *gc.C) (environs.Storage, func()) {
	listener, err := localstorage.Serve("127.0.0.1:0", c.MkDir())
	c.Assert(err, gc.IsNil)
	storage := localstorage.Client(listener.Addr().String())
	cleanup := func() { listener.Close() }
	return storage, cleanup
}

func (*StateSuite) TestCreateStateFileWritesEmptyStateFile(c *gc.C) {
	storage, cleanup := makeDummyStorage(c)
	defer cleanup()

	url, err := provider.CreateStateFile(storage)
	c.Assert(err, gc.IsNil)

	reader, err := storage.Get(provider.StateFile)
	c.Assert(err, gc.IsNil)
	data, err := ioutil.ReadAll(reader)
	c.Assert(err, gc.IsNil)
	c.Check(string(data), gc.Equals, "")
	c.Assert(url, gc.NotNil)
	expectedURL, err := storage.URL(provider.StateFile)
	c.Assert(err, gc.IsNil)
	c.Check(url, gc.Equals, expectedURL)
}

func (suite *StateSuite) TestSaveStateWritesStateFile(c *gc.C) {
	storage, cleanup := makeDummyStorage(c)
	defer cleanup()
	arch := "amd64"
	state := provider.BootstrapState{
		StateInstances:  []instance.Id{instance.Id("an-instance-id")},
		Characteristics: []instance.HardwareCharacteristics{{Arch: &arch}}}
	marshaledState, err := goyaml.Marshal(state)
	c.Assert(err, gc.IsNil)

	err = provider.SaveState(storage, &state)
	c.Assert(err, gc.IsNil)

	loadedState, err := storage.Get(provider.StateFile)
	c.Assert(err, gc.IsNil)
	content, err := ioutil.ReadAll(loadedState)
	c.Assert(err, gc.IsNil)
	c.Check(content, gc.DeepEquals, marshaledState)
}

func (suite *StateSuite) setUpSavedState(c *gc.C, storage environs.Storage) provider.BootstrapState {
	arch := "amd64"
	state := provider.BootstrapState{
		StateInstances:  []instance.Id{instance.Id("an-instance-id")},
		Characteristics: []instance.HardwareCharacteristics{{Arch: &arch}}}
	content, err := goyaml.Marshal(state)
	c.Assert(err, gc.IsNil)
	err = storage.Put(provider.StateFile, ioutil.NopCloser(bytes.NewReader(content)), int64(len(content)))
	c.Assert(err, gc.IsNil)
	return state
}

func (suite *StateSuite) TestLoadStateReadsStateFile(c *gc.C) {
	storage, cleanup := makeDummyStorage(c)
	defer cleanup()
	state := suite.setUpSavedState(c, storage)
	storedState, err := provider.LoadState(storage)
	c.Assert(err, gc.IsNil)
	c.Check(*storedState, gc.DeepEquals, state)
}

func (suite *StateSuite) TestLoadStateFromURLReadsStateFile(c *gc.C) {
	storage, cleanup := makeDummyStorage(c)
	defer cleanup()
	state := suite.setUpSavedState(c, storage)
	url, err := storage.URL(provider.StateFile)
	c.Assert(err, gc.IsNil)
	storedState, err := provider.LoadStateFromURL(url)
	c.Assert(err, gc.IsNil)
	c.Check(*storedState, gc.DeepEquals, state)
}

func (suite *StateSuite) TestLoadStateMissingFile(c *gc.C) {
	storage, cleanup := makeDummyStorage(c)
	defer cleanup()

	_, err := provider.LoadState(storage)

	c.Check(err, jc.Satisfies, errors.IsNotBootstrapped)
}

func (suite *StateSuite) TestLoadStateIntegratesWithSaveState(c *gc.C) {
	storage, cleanup := makeDummyStorage(c)
	defer cleanup()
	arch := "amd64"
	state := provider.BootstrapState{
		StateInstances:  []instance.Id{instance.Id("an-instance-id")},
		Characteristics: []instance.HardwareCharacteristics{{Arch: &arch}}}
	err := provider.SaveState(storage, &state)
	c.Assert(err, gc.IsNil)
	storedState, err := provider.LoadState(storage)
	c.Assert(err, gc.IsNil)

	c.Check(*storedState, gc.DeepEquals, state)
}

func (suite *StateSuite) TestGetDNSNamesAcceptsNil(c *gc.C) {
	result := provider.GetDNSNames(nil)
	c.Check(result, gc.DeepEquals, []string{})
}

func (suite *StateSuite) TestGetDNSNamesReturnsNames(c *gc.C) {
	instances := []instance.Instance{
		&dnsNameFakeInstance{name: "foo"},
		&dnsNameFakeInstance{name: "bar"},
	}

	c.Check(provider.GetDNSNames(instances), gc.DeepEquals, []string{"foo", "bar"})
}

func (suite *StateSuite) TestGetDNSNamesIgnoresNils(c *gc.C) {
	c.Check(provider.GetDNSNames([]instance.Instance{nil, nil}), gc.DeepEquals, []string{})
}

func (suite *StateSuite) TestGetDNSNamesIgnoresInstancesWithoutNames(c *gc.C) {
	instances := []instance.Instance{&dnsNameFakeInstance{err: instance.ErrNoDNSName}}
	c.Check(provider.GetDNSNames(instances), gc.DeepEquals, []string{})
}

func (suite *StateSuite) TestGetDNSNamesIgnoresInstancesWithBlankNames(c *gc.C) {
	instances := []instance.Instance{&dnsNameFakeInstance{name: ""}}
	c.Check(provider.GetDNSNames(instances), gc.DeepEquals, []string{})
}

func (suite *StateSuite) TestComposeAddressesAcceptsNil(c *gc.C) {
	c.Check(provider.ComposeAddresses(nil, 1433), gc.DeepEquals, []string{})
}

func (suite *StateSuite) TestComposeAddressesSuffixesAddresses(c *gc.C) {
	c.Check(
		provider.ComposeAddresses([]string{"onehost", "otherhost"}, 1957),
		gc.DeepEquals,
		[]string{"onehost:1957", "otherhost:1957"})
}

func (suite *StateSuite) TestGetStateInfo(c *gc.C) {
	cert := testing.CACert
	attrs := envtesting.FakeConfig.Merge(testing.Attrs{
		"ca-cert":    cert,
		"state-port": 123,
		"api-port":   456,
	})
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	hostnames := []string{"onehost", "otherhost"}

	stateInfo, apiInfo := provider.GetStateInfo(cfg, hostnames)

	c.Check(stateInfo.Addrs, gc.DeepEquals, []string{"onehost:123", "otherhost:123"})
	c.Check(string(stateInfo.CACert), gc.Equals, cert)
	c.Check(apiInfo.Addrs, gc.DeepEquals, []string{"onehost:456", "otherhost:456"})
	c.Check(string(apiInfo.CACert), gc.Equals, cert)
}
