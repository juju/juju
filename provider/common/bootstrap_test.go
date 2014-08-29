// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"
	"os"
	"time"

	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/cloudinit"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/storage"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/utils/ssh"
	"github.com/juju/juju/version"
)

type BootstrapSuite struct {
	coretesting.FakeJujuHomeSuite
	envtesting.ToolsFixture
}

var _ = gc.Suite(&BootstrapSuite{})

type cleaner interface {
	AddCleanup(testing.CleanupFunc)
}

func (s *BootstrapSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
	s.PatchValue(common.ConnectSSH, func(_ ssh.Client, host, checkHostScript string) error {
		return fmt.Errorf("mock connection failure to %s", host)
	})
}

func (s *BootstrapSuite) TearDownTest(c *gc.C) {
	s.ToolsFixture.TearDownTest(c)
	s.FakeJujuHomeSuite.TearDownTest(c)
}

func newStorage(suite cleaner, c *gc.C) storage.Storage {
	closer, stor, _ := envtesting.CreateLocalTestStorage(c)
	suite.AddCleanup(func(*gc.C) { closer.Close() })
	envtesting.UploadFakeTools(c, stor)
	return stor
}

func minimalConfig(c *gc.C) *config.Config {
	attrs := map[string]interface{}{
		"name":            "whatever",
		"type":            "anything, really",
		"ca-cert":         coretesting.CACert,
		"ca-private-key":  coretesting.CAKey,
		"authorized-keys": coretesting.FakeAuthKeys,
		"default-series":  version.Current.Series,
	}
	cfg, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, gc.IsNil)
	return cfg
}

func configGetter(c *gc.C) configFunc {
	cfg := minimalConfig(c)
	return func() *config.Config { return cfg }
}

func (s *BootstrapSuite) TestCannotStartInstance(c *gc.C) {
	checkPlacement := "directive"
	checkCons := constraints.MustParse("mem=8G")
	env := &mockEnviron{
		storage: newStorage(s, c),
		config:  configGetter(c),
	}

	startInstance := func(
		placement string,
		cons constraints.Value,
		_ []string,
		possibleTools tools.List,
		mcfg *cloudinit.MachineConfig,
	) (instance.Instance, *instance.HardwareCharacteristics, []network.Info, error) {
		c.Assert(placement, gc.DeepEquals, checkPlacement)
		c.Assert(cons, gc.DeepEquals, checkCons)

		// The machine config should set its upgrade behavior based on
		// the environment config.
		expectedMcfg, err := environs.NewBootstrapMachineConfig(cons, mcfg.Series)
		c.Assert(err, gc.IsNil)
		expectedMcfg.EnableOSRefreshUpdate = env.Config().EnableOSRefreshUpdate()
		expectedMcfg.EnableOSUpgrade = env.Config().EnableOSUpgrade()

		c.Assert(mcfg, gc.DeepEquals, expectedMcfg)
		return nil, nil, nil, fmt.Errorf("meh, not started")
	}

	env.startInstance = startInstance

	ctx := coretesting.Context(c)
	_, _, _, err := common.Bootstrap(ctx, env, environs.BootstrapParams{
		Constraints:    checkCons,
		Placement:      checkPlacement,
		AvailableTools: tools.List{&tools.Tools{Version: version.Current}},
	})
	c.Assert(err, gc.ErrorMatches, "cannot start bootstrap instance: meh, not started")
}

func (s *BootstrapSuite) TestCannotRecordStartedInstance(c *gc.C) {
	innerStorage := newStorage(s, c)
	stor := &mockStorage{Storage: innerStorage}

	startInstance := func(
		_ string, _ constraints.Value, _ []string, _ tools.List, _ *cloudinit.MachineConfig,
	) (
		instance.Instance, *instance.HardwareCharacteristics, []network.Info, error,
	) {
		stor.putErr = fmt.Errorf("suddenly a wild blah")
		return &mockInstance{id: "i-blah"}, nil, nil, nil
	}

	var stopped []instance.Id
	stopInstances := func(ids []instance.Id) error {
		stopped = append(stopped, ids...)
		return nil
	}

	env := &mockEnviron{
		storage:       stor,
		startInstance: startInstance,
		stopInstances: stopInstances,
		config:        configGetter(c),
	}

	ctx := coretesting.Context(c)
	_, _, _, err := common.Bootstrap(ctx, env, environs.BootstrapParams{
		AvailableTools: tools.List{&tools.Tools{Version: version.Current}},
	})
	c.Assert(err, gc.ErrorMatches, "cannot save state: suddenly a wild blah")
	c.Assert(stopped, gc.HasLen, 1)
	c.Assert(stopped[0], gc.Equals, instance.Id("i-blah"))
}

func (s *BootstrapSuite) TestCannotRecordThenCannotStop(c *gc.C) {
	innerStorage := newStorage(s, c)
	stor := &mockStorage{Storage: innerStorage}

	startInstance := func(
		_ string, _ constraints.Value, _ []string, _ tools.List, _ *cloudinit.MachineConfig,
	) (
		instance.Instance, *instance.HardwareCharacteristics, []network.Info, error,
	) {
		stor.putErr = fmt.Errorf("suddenly a wild blah")
		return &mockInstance{id: "i-blah"}, nil, nil, nil
	}

	var stopped []instance.Id
	stopInstances := func(instances []instance.Id) error {
		stopped = append(stopped, instances...)
		return fmt.Errorf("bork bork borken")
	}

	var tw loggo.TestWriter
	c.Assert(loggo.RegisterWriter("bootstrap-tester", &tw, loggo.DEBUG), gc.IsNil)
	defer loggo.RemoveWriter("bootstrap-tester")

	env := &mockEnviron{
		storage:       stor,
		startInstance: startInstance,
		stopInstances: stopInstances,
		config:        configGetter(c),
	}

	ctx := coretesting.Context(c)
	_, _, _, err := common.Bootstrap(ctx, env, environs.BootstrapParams{
		AvailableTools: tools.List{&tools.Tools{Version: version.Current}},
	})
	c.Assert(err, gc.ErrorMatches, "cannot save state: suddenly a wild blah")
	c.Assert(stopped, gc.HasLen, 1)
	c.Assert(stopped[0], gc.Equals, instance.Id("i-blah"))
	c.Assert(tw.Log(), jc.LogMatches, []jc.SimpleMessage{{
		loggo.ERROR, `cannot stop failed bootstrap instance "i-blah": bork bork borken`,
	}})
}

func (s *BootstrapSuite) TestSuccess(c *gc.C) {
	stor := newStorage(s, c)
	checkInstanceId := "i-success"
	checkHardware := instance.MustParseHardware("arch=ppc64el mem=2T")

	startInstance := func(
		_ string, _ constraints.Value, _ []string, _ tools.List, mcfg *cloudinit.MachineConfig,
	) (
		instance.Instance, *instance.HardwareCharacteristics, []network.Info, error,
	) {
		return &mockInstance{id: checkInstanceId}, &checkHardware, nil, nil
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

	env := &mockEnviron{
		storage:       stor,
		startInstance: startInstance,
		config:        getConfig,
		setConfig:     setConfig,
	}
	ctx := coretesting.Context(c)
	arch, series, _, err := common.Bootstrap(ctx, env, environs.BootstrapParams{
		AvailableTools: tools.List{&tools.Tools{Version: version.Current}},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(arch, gc.Equals, "ppc64el") // based on hardware characteristics
	c.Assert(series, gc.Equals, config.PreferredSeries(mocksConfig))
}

type neverRefreshes struct {
}

func (neverRefreshes) Refresh() error {
	return nil
}

type neverAddresses struct {
	neverRefreshes
}

func (neverAddresses) Addresses() ([]network.Address, error) {
	return nil, nil
}

var testSSHTimeout = config.SSHTimeoutOpts{
	Timeout:        coretesting.ShortWait,
	RetryDelay:     1 * time.Millisecond,
	AddressesDelay: 1 * time.Millisecond,
}

func (s *BootstrapSuite) TestWaitSSHTimesOutWaitingForAddresses(c *gc.C) {
	ctx := coretesting.Context(c)
	_, err := common.WaitSSH(ctx, nil, ssh.DefaultClient, "/bin/true", neverAddresses{}, testSSHTimeout)
	c.Check(err, gc.ErrorMatches, `waited for `+testSSHTimeout.Timeout.String()+` without getting any addresses`)
	c.Check(coretesting.Stderr(ctx), gc.Matches, "Waiting for address\n")
}

func (s *BootstrapSuite) TestWaitSSHKilledWaitingForAddresses(c *gc.C) {
	ctx := coretesting.Context(c)
	interrupted := make(chan os.Signal, 1)
	interrupted <- os.Interrupt
	_, err := common.WaitSSH(ctx, interrupted, ssh.DefaultClient, "/bin/true", neverAddresses{}, testSSHTimeout)
	c.Check(err, gc.ErrorMatches, "interrupted")
	c.Check(coretesting.Stderr(ctx), gc.Matches, "Waiting for address\n")
}

type brokenAddresses struct {
	neverRefreshes
}

func (brokenAddresses) Addresses() ([]network.Address, error) {
	return nil, fmt.Errorf("Addresses will never work")
}

func (s *BootstrapSuite) TestWaitSSHStopsOnBadError(c *gc.C) {
	ctx := coretesting.Context(c)
	_, err := common.WaitSSH(ctx, nil, ssh.DefaultClient, "/bin/true", brokenAddresses{}, testSSHTimeout)
	c.Check(err, gc.ErrorMatches, "getting addresses: Addresses will never work")
	c.Check(coretesting.Stderr(ctx), gc.Equals, "Waiting for address\n")
}

type neverOpensPort struct {
	neverRefreshes
	addr string
}

func (n *neverOpensPort) Addresses() ([]network.Address, error) {
	return network.NewAddresses(n.addr), nil
}

func (s *BootstrapSuite) TestWaitSSHTimesOutWaitingForDial(c *gc.C) {
	ctx := coretesting.Context(c)
	// 0.x.y.z addresses are always invalid
	_, err := common.WaitSSH(ctx, nil, ssh.DefaultClient, "/bin/true", &neverOpensPort{addr: "0.1.2.3"}, testSSHTimeout)
	c.Check(err, gc.ErrorMatches,
		`waited for `+testSSHTimeout.Timeout.String()+` without being able to connect: mock connection failure to 0.1.2.3`)
	c.Check(coretesting.Stderr(ctx), gc.Matches,
		"Waiting for address\n"+
			"(Attempting to connect to 0.1.2.3:22\n)+")
}

type interruptOnDial struct {
	neverRefreshes
	name        string
	interrupted chan os.Signal
	returned    bool
}

func (i *interruptOnDial) Addresses() ([]network.Address, error) {
	// kill the tomb the second time Addresses is called
	if !i.returned {
		i.returned = true
	} else {
		i.interrupted <- os.Interrupt
	}
	return []network.Address{network.NewAddress(i.name, network.ScopeUnknown)}, nil
}

func (s *BootstrapSuite) TestWaitSSHKilledWaitingForDial(c *gc.C) {
	ctx := coretesting.Context(c)
	timeout := testSSHTimeout
	timeout.Timeout = 1 * time.Minute
	interrupted := make(chan os.Signal, 1)
	_, err := common.WaitSSH(ctx, interrupted, ssh.DefaultClient, "", &interruptOnDial{name: "0.1.2.3", interrupted: interrupted}, timeout)
	c.Check(err, gc.ErrorMatches, "interrupted")
	// Exact timing is imprecise but it should have tried a few times before being killed
	c.Check(coretesting.Stderr(ctx), gc.Matches,
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

func (ac *addressesChange) Addresses() ([]network.Address, error) {
	var addrs []network.Address
	for _, addr := range ac.addrs[0] {
		addrs = append(addrs, network.NewAddress(addr, network.ScopeUnknown))
	}
	return addrs, nil
}

func (s *BootstrapSuite) TestWaitSSHRefreshAddresses(c *gc.C) {
	ctx := coretesting.Context(c)
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
	stderr := coretesting.Stderr(ctx)
	c.Check(stderr, gc.Matches,
		"Waiting for address\n"+
			"(.|\n)*(Attempting to connect to 0.1.2.3:22\n)+(.|\n)*")
	c.Check(stderr, gc.Matches,
		"Waiting for address\n"+
			"(.|\n)*(Attempting to connect to 0.1.2.4:22\n)+(.|\n)*")
}
