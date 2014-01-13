// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"bytes"
	"fmt"
	"os"
	"time"

	gc "launchpad.net/gocheck"
	"launchpad.net/loggo"

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
	"launchpad.net/juju-core/utils/ssh"
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
	s.PatchValue(common.ConnectSSH, func(_ ssh.Client, host, checkHostScript string) error {
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

// bootstrapContext creates a BootstrapContext which
// writes stderr to the bytes.Buffer returned.
func bootstrapContext(c *gc.C) (ctx environs.BootstrapContext, stderr *bytes.Buffer) {
	cmdContext := coretesting.Context(c)
	stderr = &bytes.Buffer{}
	cmdContext.Stderr = stderr
	return envtesting.NewBootstrapContext(cmdContext), stderr
}

func (s *BootstrapSuite) TestCannotWriteStateFile(c *gc.C) {
	brokenStorage := &mockStorage{
		Storage: newStorage(s, c),
		putErr:  fmt.Errorf("noes!"),
	}
	env := &mockEnviron{storage: brokenStorage}
	ctx, _ := bootstrapContext(c)
	err := common.Bootstrap(ctx, env, constraints.Value{})
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
		c.Assert(mcfg, gc.DeepEquals, environs.NewBootstrapMachineConfig(checkURL, mcfg.SystemPrivateSSHKey))
		return nil, nil, fmt.Errorf("meh, not started")
	}

	env := &mockEnviron{
		storage:       stor,
		startInstance: startInstance,
		config:        configGetter(c),
	}

	ctx, _ := bootstrapContext(c)
	err = common.Bootstrap(ctx, env, checkCons)
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

	ctx, _ := bootstrapContext(c)
	err := common.Bootstrap(ctx, env, constraints.Value{})
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

	ctx, _ := bootstrapContext(c)
	err := common.Bootstrap(ctx, env, constraints.Value{})
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
	var mocksConfig = minimalConfig(c)
	var getConfigCalled int
	getConfig := func() *config.Config {
		getConfigCalled++
		return mocksConfig
	}
	setConfig := func(c *config.Config) error {
		mocksConfig = c
		return nil
	}

	restore := envtesting.DisableFinishBootstrap()
	defer restore()

	env := &mockEnviron{
		storage:       stor,
		startInstance: startInstance,
		config:        getConfig,
		setConfig:     setConfig,
	}
	originalAuthKeys := env.Config().AuthorizedKeys()
	ctx, _ := bootstrapContext(c)
	err := common.Bootstrap(ctx, env, constraints.Value{})
	c.Assert(err, gc.IsNil)

	savedState, err := bootstrap.LoadStateFromURL(checkURL)
	c.Assert(err, gc.IsNil)
	c.Assert(savedState, gc.DeepEquals, &bootstrap.BootstrapState{
		StateInstances:  []instance.Id{instance.Id(checkInstanceId)},
		Characteristics: []instance.HardwareCharacteristics{checkHardware},
	})
	authKeys := env.Config().AuthorizedKeys()
	c.Assert(authKeys, gc.Not(gc.Equals), originalAuthKeys)
	c.Assert(authKeys, jc.HasSuffix, "juju-system-key\n")
}

type neverRefreshes struct {
}

func (neverRefreshes) Refresh() error {
	return nil
}

type neverAddresses struct {
	neverRefreshes
}

func (neverAddresses) Addresses() ([]instance.Address, error) {
	return nil, nil
}

var testSSHTimeout = common.SSHTimeoutOpts{
	Timeout:        coretesting.ShortWait,
	ConnectDelay:   1 * time.Millisecond,
	AddressesDelay: 1 * time.Millisecond,
}

func (s *BootstrapSuite) TestWaitSSHTimesOutWaitingForAddresses(c *gc.C) {
	ctx, stderr := bootstrapContext(c)
	_, err := common.WaitSSH(ctx, nil, ssh.DefaultClient, "/bin/true", neverAddresses{}, testSSHTimeout)
	c.Check(err, gc.ErrorMatches, `waited for `+testSSHTimeout.Timeout.String()+` without getting any addresses`)
	c.Check(stderr.String(), gc.Matches, "Waiting for address\n")
}

func (s *BootstrapSuite) TestWaitSSHKilledWaitingForAddresses(c *gc.C) {
	ctx, stderr := bootstrapContext(c)
	interrupted := make(chan os.Signal, 1)
	go func() {
		<-time.After(2 * time.Millisecond)
		interrupted <- os.Interrupt
	}()
	_, err := common.WaitSSH(ctx, interrupted, ssh.DefaultClient, "/bin/true", neverAddresses{}, testSSHTimeout)
	c.Check(err, gc.ErrorMatches, "interrupted")
	c.Check(stderr.String(), gc.Matches, "Waiting for address\n")
}

type brokenAddresses struct {
	neverRefreshes
}

func (brokenAddresses) Addresses() ([]instance.Address, error) {
	return nil, fmt.Errorf("Addresses will never work")
}

func (s *BootstrapSuite) TestWaitSSHStopsOnBadError(c *gc.C) {
	ctx, stderr := bootstrapContext(c)
	_, err := common.WaitSSH(ctx, nil, ssh.DefaultClient, "/bin/true", brokenAddresses{}, testSSHTimeout)
	c.Check(err, gc.ErrorMatches, "getting addresses: Addresses will never work")
	c.Check(stderr.String(), gc.Equals, "Waiting for address\n")
}

type neverOpensPort struct {
	neverRefreshes
	addr string
}

func (n *neverOpensPort) Addresses() ([]instance.Address, error) {
	return []instance.Address{instance.NewAddress(n.addr)}, nil
}

func (s *BootstrapSuite) TestWaitSSHTimesOutWaitingForDial(c *gc.C) {
	ctx, stderr := bootstrapContext(c)
	// 0.x.y.z addresses are always invalid
	_, err := common.WaitSSH(ctx, nil, ssh.DefaultClient, "/bin/true", &neverOpensPort{addr: "0.1.2.3"}, testSSHTimeout)
	c.Check(err, gc.ErrorMatches,
		`waited for `+testSSHTimeout.Timeout.String()+` without being able to connect: mock connection failure to 0.1.2.3`)
	c.Check(stderr.String(), gc.Matches,
		"Waiting for address\n"+
			"(Attempting to connect to 0.1.2.3:22\n)+")
}

type interruptOnDial struct {
	neverRefreshes
	name        string
	interrupted chan os.Signal
	returned    bool
}

func (i *interruptOnDial) Addresses() ([]instance.Address, error) {
	// kill the tomb the second time Addresses is called
	if !i.returned {
		i.returned = true
	} else {
		i.interrupted <- os.Interrupt
	}
	return []instance.Address{instance.NewAddress(i.name)}, nil
}

func (s *BootstrapSuite) TestWaitSSHKilledWaitingForDial(c *gc.C) {
	ctx, stderr := bootstrapContext(c)
	timeout := testSSHTimeout
	timeout.Timeout = 1 * time.Minute
	interrupted := make(chan os.Signal, 1)
	_, err := common.WaitSSH(ctx, interrupted, ssh.DefaultClient, "", &interruptOnDial{name: "0.1.2.3", interrupted: interrupted}, timeout)
	c.Check(err, gc.ErrorMatches, "interrupted")
	// Exact timing is imprecise but it should have tried a few times before being killed
	c.Check(stderr.String(), gc.Matches,
		"Waiting for address\n"+
			"(Attempting to connect to 0.1.2.3:22\n)+")
}

type addressesChange struct {
	addrs [][]string
}

func (ac *addressesChange) Refresh() error {
	if len(ac.addrs) > 1 {
		ac.addrs = ac.addrs[1:]
	}
	return nil
}

func (ac *addressesChange) Addresses() ([]instance.Address, error) {
	var addrs []instance.Address
	for _, addr := range ac.addrs[0] {
		addrs = append(addrs, instance.NewAddress(addr))
	}
	return addrs, nil
}

func (s *BootstrapSuite) TestWaitSSHRefreshAddresses(c *gc.C) {
	ctx, stderr := bootstrapContext(c)
	_, err := common.WaitSSH(ctx, nil, ssh.DefaultClient, "", &addressesChange{addrs: [][]string{
		nil,
		nil,
		[]string{"0.1.2.3"},
		[]string{"0.1.2.3"},
		nil,
		[]string{"0.1.2.4"},
	}}, testSSHTimeout)
	// Not necessarily the last one in the list, due to scheduling.
	c.Check(err, gc.ErrorMatches,
		`waited for `+testSSHTimeout.Timeout.String()+` without being able to connect: mock connection failure to 0.1.2.[34]`)
	c.Check(stderr.String(), gc.Matches,
		"Waiting for address\n"+
			"(.|\n)*(Attempting to connect to 0.1.2.3:22\n)+(.|\n)*")
	c.Check(stderr.String(), gc.Matches,
		"Waiting for address\n"+
			"(.|\n)*(Attempting to connect to 0.1.2.4:22\n)+(.|\n)*")
}
