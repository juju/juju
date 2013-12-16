// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"bytes"
	"fmt"
	"time"

	gc "launchpad.net/gocheck"
	"launchpad.net/loggo"
	"launchpad.net/tomb"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/bootstrap"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/storage"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/provider/common"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/tools"
)

type BootstrapSuite struct {
	testbase.LoggingSuite
	envtesting.ToolsFixture
}

var _ = gc.Suite(&BootstrapSuite{})

type cleaner interface {
	AddCleanup(testbase.CleanupFunc)
}

func (s *BootstrapSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
	s.PatchValue(common.ConnectSSH, func(host, checkHostScript string) error {
		return fmt.Errorf("mock connection failure to %s", host)
	})
}

func (s *BootstrapSuite) TearDownTest(c *gc.C) {
	s.ToolsFixture.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

func newStorage(suite cleaner, c *gc.C) storage.Storage {
	closer, stor, _ := envtesting.CreateLocalTestStorage(c)
	suite.AddCleanup(func(*gc.C) { closer.Close() })
	envtesting.UploadFakeTools(c, stor)
	return stor
}

func minimalConfig(c *gc.C) *config.Config {
	attrs := map[string]interface{}{
		"name":           "whatever",
		"type":           "anything, really",
		"ca-cert":        coretesting.CACert,
		"ca-private-key": coretesting.CAKey,
	}
	cfg, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, gc.IsNil)
	return cfg
}

func configGetter(c *gc.C) configFunc {
	cfg := minimalConfig(c)
	return func() *config.Config { return cfg }
}

func (s *BootstrapSuite) TestCannotWriteStateFile(c *gc.C) {
	brokenStorage := &mockStorage{
		Storage: newStorage(s, c),
		putErr:  fmt.Errorf("noes!"),
	}
	env := &mockEnviron{storage: brokenStorage}
	err := common.Bootstrap(env, constraints.Value{})
	c.Assert(err, gc.ErrorMatches, "cannot create initial state file: noes!")
}

func (s *BootstrapSuite) TestCannotStartInstance(c *gc.C) {
	stor := newStorage(s, c)
	checkURL, err := stor.URL(bootstrap.StateFile)
	c.Assert(err, gc.IsNil)
	checkCons := constraints.MustParse("mem=8G")

	startInstance := func(
		cons constraints.Value, possibleTools tools.List, mcfg *cloudinit.MachineConfig,
	) (
		instance.Instance, *instance.HardwareCharacteristics, error,
	) {
		c.Assert(cons, gc.DeepEquals, checkCons)
		c.Assert(mcfg, gc.DeepEquals, environs.NewBootstrapMachineConfig(checkURL))
		return nil, nil, fmt.Errorf("meh, not started")
	}

	env := &mockEnviron{
		storage:       stor,
		startInstance: startInstance,
		config:        configGetter(c),
	}

	err = common.Bootstrap(env, checkCons)
	c.Assert(err, gc.ErrorMatches, "cannot start bootstrap instance: meh, not started")
}

func (s *BootstrapSuite) TestCannotRecordStartedInstance(c *gc.C) {
	innerStorage := newStorage(s, c)
	stor := &mockStorage{Storage: innerStorage}

	startInstance := func(
		_ constraints.Value, _ tools.List, _ *cloudinit.MachineConfig,
	) (
		instance.Instance, *instance.HardwareCharacteristics, error,
	) {
		stor.putErr = fmt.Errorf("suddenly a wild blah")
		return &mockInstance{id: "i-blah"}, nil, nil
	}

	var stopped []instance.Instance
	stopInstances := func(instances []instance.Instance) error {
		stopped = append(stopped, instances...)
		return nil
	}

	env := &mockEnviron{
		storage:       stor,
		startInstance: startInstance,
		stopInstances: stopInstances,
		config:        configGetter(c),
	}

	err := common.Bootstrap(env, constraints.Value{})
	c.Assert(err, gc.ErrorMatches, "cannot save state: suddenly a wild blah")
	c.Assert(stopped, gc.HasLen, 1)
	c.Assert(stopped[0].Id(), gc.Equals, instance.Id("i-blah"))
}

func (s *BootstrapSuite) TestCannotRecordThenCannotStop(c *gc.C) {
	innerStorage := newStorage(s, c)
	stor := &mockStorage{Storage: innerStorage}

	startInstance := func(
		_ constraints.Value, _ tools.List, _ *cloudinit.MachineConfig,
	) (
		instance.Instance, *instance.HardwareCharacteristics, error,
	) {
		stor.putErr = fmt.Errorf("suddenly a wild blah")
		return &mockInstance{id: "i-blah"}, nil, nil
	}

	var stopped []instance.Instance
	stopInstances := func(instances []instance.Instance) error {
		stopped = append(stopped, instances...)
		return fmt.Errorf("bork bork borken")
	}

	tw := &loggo.TestWriter{}
	c.Assert(loggo.RegisterWriter("bootstrap-tester", tw, loggo.DEBUG), gc.IsNil)
	defer loggo.RemoveWriter("bootstrap-tester")

	env := &mockEnviron{
		storage:       stor,
		startInstance: startInstance,
		stopInstances: stopInstances,
		config:        configGetter(c),
	}

	err := common.Bootstrap(env, constraints.Value{})
	c.Assert(err, gc.ErrorMatches, "cannot save state: suddenly a wild blah")
	c.Assert(stopped, gc.HasLen, 1)
	c.Assert(stopped[0].Id(), gc.Equals, instance.Id("i-blah"))
	c.Assert(tw.Log, jc.LogMatches, []jc.SimpleMessage{{
		loggo.ERROR, `cannot stop failed bootstrap instance "i-blah": bork bork borken`,
	}})
}

func (s *BootstrapSuite) TestSuccess(c *gc.C) {
	stor := newStorage(s, c)
	checkInstanceId := "i-success"
	checkHardware := instance.MustParseHardware("mem=2T")

	checkURL := ""
	startInstance := func(
		_ constraints.Value, _ tools.List, mcfg *cloudinit.MachineConfig,
	) (
		instance.Instance, *instance.HardwareCharacteristics, error,
	) {
		checkURL = mcfg.StateInfoURL
		return &mockInstance{id: checkInstanceId}, &checkHardware, nil
	}

	var getConfigCalled int
	getConfig := func() *config.Config {
		getConfigCalled++
		return minimalConfig(c)
	}

	restore := envtesting.DisableFinishBootstrap()
	defer restore()

	env := &mockEnviron{
		storage:       stor,
		startInstance: startInstance,
		config:        getConfig,
	}
	err := common.Bootstrap(env, constraints.Value{})
	c.Assert(err, gc.IsNil)

	savedState, err := bootstrap.LoadStateFromURL(checkURL)
	c.Assert(err, gc.IsNil)
	c.Assert(savedState, gc.DeepEquals, &bootstrap.BootstrapState{
		StateInstances:  []instance.Id{instance.Id(checkInstanceId)},
		Characteristics: []instance.HardwareCharacteristics{checkHardware},
	})
}

type neverAddresses struct {
}

func (neverAddresses) Addresses() ([]instance.Address, error) {
	return nil, nil
}

var testSSHTimeout = common.SSHTimeoutOpts{
	Timeout:        10 * time.Millisecond,
	ConnectDelay:   1 * time.Millisecond,
	AddressesDelay: 1 * time.Millisecond,
}

func (s *BootstrapSuite) TestWaitSSHTimesOutWaitingForAddresses(c *gc.C) {
	ctx := &common.BootstrapContext{}
	buf := &bytes.Buffer{}
	ctx.Stderr = buf
	var t tomb.Tomb
	_, err := common.WaitSSH(ctx, "/bin/true", neverAddresses{}, &t, testSSHTimeout)
	c.Check(err, gc.ErrorMatches, "waited for 10ms without getting any addresses")
	c.Check(buf.String(), gc.Matches, "Waiting for address\n")
}

func (s *BootstrapSuite) TestWaitSSHKilledWaitingForAddresses(c *gc.C) {
	ctx := &common.BootstrapContext{}
	buf := &bytes.Buffer{}
	ctx.Stderr = buf
	var t tomb.Tomb
	go func() {
		<-time.After(2 * time.Millisecond)
		t.Killf("stopping WaitSSH during Addresses")
	}()
	_, err := common.WaitSSH(ctx, "/bin/true", neverAddresses{}, &t, testSSHTimeout)
	c.Check(err, gc.ErrorMatches, "stopping WaitSSH during Addresses")
	c.Check(buf.String(), gc.Matches, "Waiting for address\n")
}

type brokenAddresses struct {
}

func (brokenAddresses) Addresses() ([]instance.Address, error) {
	return nil, fmt.Errorf("Addresses will never work")
}

func (s *BootstrapSuite) TestWaitSSHStopsOnBadError(c *gc.C) {
	ctx := &common.BootstrapContext{}
	buf := &bytes.Buffer{}
	ctx.Stderr = buf
	var t tomb.Tomb
	_, err := common.WaitSSH(ctx, "/bin/true", brokenAddresses{}, &t, testSSHTimeout)
	c.Check(err, gc.ErrorMatches, "getting addresses: Addresses will never work")
	c.Check(buf.String(), gc.Equals, "Waiting for address\n")
}

type neverOpensPort struct {
	name string
}

func (n *neverOpensPort) Addresses() ([]instance.Address, error) {
	return []instance.Address{instance.NewAddress(n.name)}, nil
}

func (s *BootstrapSuite) TestWaitSSHTimesOutWaitingForDial(c *gc.C) {
	ctx := &common.BootstrapContext{}
	buf := &bytes.Buffer{}
	ctx.Stderr = buf
	var t tomb.Tomb
	// 0.x.y.z addresses are always invalid
	_, err := common.WaitSSH(ctx, "/bin/true", &neverOpensPort{"0.1.2.3"}, &t, testSSHTimeout)
	c.Check(err, gc.ErrorMatches,
		`waited for 10ms without being able to connect: mock connection failure to 0.1.2.3`)
	c.Check(buf.String(), gc.Matches,
		"Waiting for address\n"+
			"(Attempting to connect to 0.1.2.3:22\n)+")
}

type killOnDial struct {
	name     string
	tomb     *tomb.Tomb
	returned bool
}

func (k *killOnDial) Addresses() ([]instance.Address, error) {
	// kill the tomb the second time Addresses is called
	if !k.returned {
		k.returned = true
	} else {
		k.tomb.Killf("stopping WaitSSH during Dial")
	}
	return []instance.Address{instance.NewAddress(k.name)}, nil
}

func (s *BootstrapSuite) TestWaitSSHKilledWaitingForDial(c *gc.C) {
	ctx := &common.BootstrapContext{}
	buf := &bytes.Buffer{}
	ctx.Stderr = buf
	var t tomb.Tomb
	timeout := testSSHTimeout
	timeout.Timeout = 1 * time.Minute
	_, err := common.WaitSSH(ctx, "", &killOnDial{name: "0.1.2.3", tomb: &t}, &t, timeout)
	c.Check(err, gc.ErrorMatches, "stopping WaitSSH during Dial")
	// Exact timing is imprecise but it should have tried a few times before being killed
	c.Check(buf.String(), gc.Matches,
		"Waiting for address\n"+
			"(Attempting to connect to 0.1.2.3:22\n)+")
}

type addressesChanges struct {
	names []string
}

func (d *addressesChanges) Addresses() ([]instance.Address, error) {
	name := d.names[0]
	if len(d.names) > 1 {
		d.names = d.names[1:]
	}
	if name == "" {
		return nil, nil
	}
	return []instance.Address{instance.NewAddress(name)}, nil
}

func (s *BootstrapSuite) TestWaitSSHRefreshAddresses(c *gc.C) {
	ctx := &common.BootstrapContext{}
	buf := &bytes.Buffer{}
	ctx.Stderr = buf
	var t tomb.Tomb
	_, err := common.WaitSSH(ctx, "", &addressesChanges{[]string{"", "0.1.2.3", "0.1.2.3", "", "0.1.2.4"}}, &t, testSSHTimeout)
	// Not necessarily the last one in the list, due to scheduling.
	c.Check(err, gc.ErrorMatches,
		`waited for 10ms without being able to connect: mock connection failure to 0.1.2.[34]`)
	c.Check(buf.String(), gc.Matches,
		"Waiting for address\n"+
			"(.|\n)*(Attempting to connect to 0.1.2.3:22\n)+(.|\n)*")
	c.Check(buf.String(), gc.Matches,
		"Waiting for address\n"+
			"(.|\n)*(Attempting to connect to 0.1.2.4:22\n)+(.|\n)*")
}
