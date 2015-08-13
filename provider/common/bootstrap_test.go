// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"
	"os"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
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
	envtesting.UploadFakeTools(c, stor, "released", "released")
	return stor
}

func minimalConfig(c *gc.C) *config.Config {
	attrs := map[string]interface{}{
		"name":            "whatever",
		"type":            "anything, really",
		"uuid":            coretesting.EnvironmentTag.Id(),
		"ca-cert":         coretesting.CACert,
		"ca-private-key":  coretesting.CAKey,
		"authorized-keys": coretesting.FakeAuthKeys,
		"default-series":  version.Current.Series,
	}
	cfg, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

func configGetter(c *gc.C) configFunc {
	cfg := minimalConfig(c)
	return func() *config.Config { return cfg }
}

func (s *BootstrapSuite) TestCannotStartInstance(c *gc.C) {
	s.PatchValue(&version.Current.Number, coretesting.FakeVersionNumber)
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
		icfg *instancecfg.InstanceConfig,
	) (instance.Instance, *instance.HardwareCharacteristics, []network.InterfaceInfo, error) {
		c.Assert(placement, gc.DeepEquals, checkPlacement)
		c.Assert(cons, gc.DeepEquals, checkCons)

		// The machine config should set its upgrade behavior based on
		// the environment config.
		expectedMcfg, err := instancecfg.NewBootstrapInstanceConfig(cons, icfg.Series)
		c.Assert(err, jc.ErrorIsNil)
		expectedMcfg.EnableOSRefreshUpdate = env.Config().EnableOSRefreshUpdate()
		expectedMcfg.EnableOSUpgrade = env.Config().EnableOSUpgrade()
		expectedMcfg.Tags = map[string]string{
			"juju-env-uuid": coretesting.EnvironmentTag.Id(),
			"juju-is-state": "true",
		}

		c.Assert(icfg, jc.DeepEquals, expectedMcfg)
		return nil, nil, nil, fmt.Errorf("meh, not started")
	}

	env.startInstance = startInstance

	ctx := envtesting.BootstrapContext(c)
	_, _, _, err := common.Bootstrap(ctx, env, environs.BootstrapParams{
		Constraints:    checkCons,
		Placement:      checkPlacement,
		AvailableTools: tools.List{&tools.Tools{Version: version.Current}},
	})
	c.Assert(err, gc.ErrorMatches, "cannot start bootstrap instance: meh, not started")
}

func (s *BootstrapSuite) TestSuccess(c *gc.C) {
	s.PatchValue(&version.Current.Number, coretesting.FakeVersionNumber)
	stor := newStorage(s, c)
	checkInstanceId := "i-success"
	checkHardware := instance.MustParseHardware("arch=ppc64el mem=2T")

	startInstance := func(
		_ string, _ constraints.Value, _ []string, _ tools.List, icfg *instancecfg.InstanceConfig,
	) (
		instance.Instance, *instance.HardwareCharacteristics, []network.InterfaceInfo, error,
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
	ctx := envtesting.BootstrapContext(c)
	arch, series, _, err := common.Bootstrap(ctx, env, environs.BootstrapParams{
		AvailableTools: tools.List{&tools.Tools{Version: version.Current}},
	})
	c.Assert(err, jc.ErrorIsNil)
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
	_, err := common.WaitSSH(envcmd.BootstrapContext(ctx), nil, ssh.DefaultClient, "/bin/true", neverAddresses{}, testSSHTimeout)
	c.Check(err, gc.ErrorMatches, `waited for `+testSSHTimeout.Timeout.String()+` without getting any addresses`)
	c.Check(coretesting.Stderr(ctx), gc.Matches, "Waiting for address\n")
}

func (s *BootstrapSuite) TestWaitSSHKilledWaitingForAddresses(c *gc.C) {
	ctx := coretesting.Context(c)
	interrupted := make(chan os.Signal, 1)
	interrupted <- os.Interrupt
	_, err := common.WaitSSH(envcmd.BootstrapContext(ctx), interrupted, ssh.DefaultClient, "/bin/true", neverAddresses{}, testSSHTimeout)
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
	_, err := common.WaitSSH(envcmd.BootstrapContext(ctx), nil, ssh.DefaultClient, "/bin/true", brokenAddresses{}, testSSHTimeout)
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
	_, err := common.WaitSSH(envcmd.BootstrapContext(ctx), nil, ssh.DefaultClient, "/bin/true", &neverOpensPort{addr: "0.1.2.3"}, testSSHTimeout)
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
	return network.NewAddresses(i.name), nil
}

func (s *BootstrapSuite) TestWaitSSHKilledWaitingForDial(c *gc.C) {
	ctx := coretesting.Context(c)
	timeout := testSSHTimeout
	timeout.Timeout = 1 * time.Minute
	interrupted := make(chan os.Signal, 1)
	_, err := common.WaitSSH(envcmd.BootstrapContext(ctx), interrupted, ssh.DefaultClient, "", &interruptOnDial{name: "0.1.2.3", interrupted: interrupted}, timeout)
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
	return network.NewAddresses(ac.addrs[0]...), nil
}

func (s *BootstrapSuite) TestWaitSSHRefreshAddresses(c *gc.C) {
	ctx := coretesting.Context(c)
	_, err := common.WaitSSH(envcmd.BootstrapContext(ctx), nil, ssh.DefaultClient, "", &addressesChange{addrs: [][]string{
		nil,
		nil,
		{"0.1.2.3"},
		{"0.1.2.3"},
		nil,
		{"0.1.2.4"},
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
