// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"crypto/rsa"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/series"
	"github.com/juju/utils/ssh"
	"github.com/juju/version"
	cryptossh "golang.org/x/crypto/ssh"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/storage"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/status"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
)

type BootstrapSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	envtesting.ToolsFixture
}

var _ = gc.Suite(&BootstrapSuite{})

type cleaner interface {
	AddCleanup(func(*gc.C))
}

func (s *BootstrapSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
	s.PatchValue(common.ConnectSSH, func(_ ssh.Client, host, checkHostScript string, opts *ssh.Options) error {
		return fmt.Errorf("mock connection failure to %s", host)
	})
}

func (s *BootstrapSuite) TearDownTest(c *gc.C) {
	s.ToolsFixture.TearDownTest(c)
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
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
		"uuid":            coretesting.ModelTag.Id(),
		"controller-uuid": coretesting.ControllerTag.Id(),
		"ca-cert":         coretesting.CACert,
		"ca-private-key":  coretesting.CAKey,
		"authorized-keys": coretesting.FakeAuthKeys,
		"default-series":  series.MustHostSeries(),
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
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
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
		expectedMcfg, err := instancecfg.NewBootstrapInstanceConfig(coretesting.FakeControllerConfig(), cons, cons, icfg.Series, "")
		c.Assert(err, jc.ErrorIsNil)
		expectedMcfg.EnableOSRefreshUpdate = env.Config().EnableOSRefreshUpdate()
		expectedMcfg.EnableOSUpgrade = env.Config().EnableOSUpgrade()
		expectedMcfg.Tags = map[string]string{
			"juju-model-uuid":      coretesting.ModelTag.Id(),
			"juju-controller-uuid": coretesting.ControllerTag.Id(),
			"juju-is-controller":   "true",
		}
		expectedMcfg.NetBondReconfigureDelay = env.Config().NetBondReconfigureDelay()
		icfg.Bootstrap.InitialSSHHostKeys.RSA = nil

		c.Assert(icfg, jc.DeepEquals, expectedMcfg)
		return nil, nil, nil, errors.Errorf("meh, not started")
	}

	env.startInstance = startInstance

	ctx := envtesting.BootstrapContext(c)
	_, err := common.Bootstrap(ctx, env, environs.BootstrapParams{
		ControllerConfig:     coretesting.FakeControllerConfig(),
		BootstrapConstraints: checkCons,
		ModelConstraints:     checkCons,
		Placement:            checkPlacement,
		AvailableTools: tools.List{
			&tools.Tools{
				Version: version.Binary{
					Number: jujuversion.Current,
					Arch:   arch.HostArch(),
					Series: series.MustHostSeries(),
				},
			},
		}})
	c.Assert(err, gc.ErrorMatches, "cannot start bootstrap instance: meh, not started")
}

func (s *BootstrapSuite) TestBootstrapSeries(c *gc.C) {
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	s.PatchValue(&series.MustHostSeries, func() string { return "precise" })
	stor := newStorage(s, c)
	checkInstanceId := "i-success"
	checkHardware := instance.MustParseHardware("arch=ppc64el mem=2T")

	startInstance := func(_ string, _ constraints.Value, _ []string, _ tools.List, icfg *instancecfg.InstanceConfig) (instance.Instance,
		*instance.HardwareCharacteristics, []network.InterfaceInfo, error) {
		return &mockInstance{id: checkInstanceId}, &checkHardware, nil, nil
	}
	var mocksConfig = minimalConfig(c)
	var numGetConfigCalled int
	getConfig := func() *config.Config {
		numGetConfigCalled++
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
	bootstrapSeries := "utopic"
	result, err := common.Bootstrap(ctx, env, environs.BootstrapParams{
		ControllerConfig: coretesting.FakeControllerConfig(),
		BootstrapSeries:  bootstrapSeries,
		AvailableTools: tools.List{
			&tools.Tools{
				Version: version.Binary{
					Number: jujuversion.Current,
					Arch:   arch.HostArch(),
					Series: bootstrapSeries,
				},
			},
		}})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Arch, gc.Equals, "ppc64el") // based on hardware characteristics
	c.Check(result.Series, gc.Equals, bootstrapSeries)
}

func (s *BootstrapSuite) TestSuccess(c *gc.C) {
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	stor := newStorage(s, c)
	checkInstanceId := "i-success"
	checkHardware := instance.MustParseHardware("arch=ppc64el mem=2T")

	var innerInstanceConfig *instancecfg.InstanceConfig
	inst := &mockInstance{
		id:        checkInstanceId,
		addresses: network.NewAddresses("testing.invalid"),
	}
	startInstance := func(
		_ string, _ constraints.Value, _ []string, tools tools.List, icfg *instancecfg.InstanceConfig,
	) (
		instance.Instance, *instance.HardwareCharacteristics, []network.InterfaceInfo, error,
	) {
		innerInstanceConfig = icfg
		c.Assert(icfg.Bootstrap.InitialSSHHostKeys.RSA, gc.NotNil)
		privKey, err := cryptossh.ParseRawPrivateKey([]byte(icfg.Bootstrap.InitialSSHHostKeys.RSA.Private))
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(privKey, gc.FitsTypeOf, &rsa.PrivateKey{})
		pubKey, _, _, _, err := cryptossh.ParseAuthorizedKey([]byte(icfg.Bootstrap.InitialSSHHostKeys.RSA.Public))
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(pubKey.Type(), gc.Equals, cryptossh.KeyAlgoRSA)
		return inst, &checkHardware, nil, nil
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

	var instancesMu sync.Mutex
	env := &mockEnviron{
		storage:       stor,
		startInstance: startInstance,
		config:        getConfig,
		setConfig:     setConfig,
		instances: func(ids []instance.Id) ([]instance.Instance, error) {
			instancesMu.Lock()
			defer instancesMu.Unlock()
			return []instance.Instance{inst}, nil
		},
	}
	inner := cmdtesting.Context(c)
	ctx := modelcmd.BootstrapContext(inner)
	result, err := common.Bootstrap(ctx, env, environs.BootstrapParams{
		ControllerConfig: coretesting.FakeControllerConfig(),
		AvailableTools: tools.List{
			&tools.Tools{
				Version: version.Binary{
					Number: jujuversion.Current,
					Arch:   arch.HostArch(),
					Series: series.MustHostSeries(),
				},
			},
		}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Arch, gc.Equals, "ppc64el") // based on hardware characteristics
	c.Assert(result.Series, gc.Equals, config.PreferredSeries(mocksConfig))
	c.Assert(result.Finalize, gc.NotNil)

	// Check that we make the SSH connection with desired options.
	var knownHosts string
	re := regexp.MustCompile(
		"ssh '-o' 'StrictHostKeyChecking yes' " +
			"'-o' 'PasswordAuthentication no' " +
			"'-o' 'ServerAliveInterval 30' " +
			"'-o' 'UserKnownHostsFile (.*)' " +
			"'-o' 'HostKeyAlgorithms ssh-rsa' " +
			"'ubuntu@testing.invalid' '/bin/bash'")
	testing.PatchExecutableAsEchoArgs(c, s, "ssh")
	s.PatchValue(common.ConnectSSH, func(_ ssh.Client, host, checkHostScript string, opts *ssh.Options) error {
		// Stop WaitSSH from continuing.
		client, err := ssh.NewOpenSSHClient()
		if err != nil {
			return err
		}
		cmd := client.Command("ubuntu@"+host, []string{"/bin/bash"}, opts)
		if err := cmd.Run(); err != nil {
			return nil
		}
		sshArgs := testing.ReadEchoArgs(c, "ssh")
		submatch := re.FindStringSubmatch(sshArgs)
		if c.Check(submatch, gc.NotNil, gc.Commentf("%s", sshArgs)) {
			knownHostsFile := submatch[1]
			knownHostsBytes, err := ioutil.ReadFile(knownHostsFile)
			c.Assert(err, jc.ErrorIsNil)
			knownHosts = string(knownHostsBytes)
		}
		return nil
	})
	err = result.Finalize(ctx, innerInstanceConfig, environs.BootstrapDialOpts{
		Timeout: coretesting.LongWait,
	})
	c.Assert(err, gc.ErrorMatches, "invalid machine configuration: .*") // icfg hasn't been finalized
	c.Assert(
		string(knownHosts),
		gc.Equals,
		"testing.invalid "+innerInstanceConfig.Bootstrap.InitialSSHHostKeys.RSA.Public,
	)
}

type neverRefreshes struct {
}

func (neverRefreshes) Refresh() error {
	return nil
}

func (neverRefreshes) Status() instance.InstanceStatus {
	return instance.InstanceStatus{}
}

type neverAddresses struct {
	neverRefreshes
}

func (neverAddresses) Addresses() ([]network.Address, error) {
	return nil, nil
}

type failsProvisioning struct {
	neverAddresses
	message string
}

func (f failsProvisioning) Status() instance.InstanceStatus {
	return instance.InstanceStatus{
		Status:  status.ProvisioningError,
		Message: f.message,
	}
}

var testSSHTimeout = environs.BootstrapDialOpts{
	Timeout:        coretesting.ShortWait,
	RetryDelay:     1 * time.Millisecond,
	AddressesDelay: 1 * time.Millisecond,
}

func (s *BootstrapSuite) TestWaitSSHTimesOutWaitingForAddresses(c *gc.C) {
	ctx := cmdtesting.Context(c)
	_, err := common.WaitSSH(
		ctx.Stderr, nil, ssh.DefaultClient, "/bin/true", neverAddresses{}, testSSHTimeout,
		common.DefaultHostSSHOptions,
	)
	c.Check(err, gc.ErrorMatches, `waited for `+testSSHTimeout.Timeout.String()+` without getting any addresses`)
	c.Check(cmdtesting.Stderr(ctx), gc.Matches, "Waiting for address\n")
}

func (s *BootstrapSuite) TestWaitSSHKilledWaitingForAddresses(c *gc.C) {
	ctx := cmdtesting.Context(c)
	interrupted := make(chan os.Signal, 1)
	interrupted <- os.Interrupt
	_, err := common.WaitSSH(
		ctx.Stderr, interrupted, ssh.DefaultClient, "/bin/true", neverAddresses{}, testSSHTimeout,
		common.DefaultHostSSHOptions,
	)
	c.Check(err, gc.ErrorMatches, "interrupted")
	c.Check(cmdtesting.Stderr(ctx), gc.Matches, "Waiting for address\n")
}

func (s *BootstrapSuite) TestWaitSSHNoticesProvisioningFailures(c *gc.C) {
	ctx := cmdtesting.Context(c)
	_, err := common.WaitSSH(
		ctx.Stderr, nil, ssh.DefaultClient, "/bin/true", failsProvisioning{}, testSSHTimeout,
		common.DefaultHostSSHOptions,
	)
	c.Check(err, gc.ErrorMatches, `instance provisioning failed`)
	_, err = common.WaitSSH(
		ctx.Stderr, nil, ssh.DefaultClient, "/bin/true", failsProvisioning{message: "blargh"}, testSSHTimeout,
		common.DefaultHostSSHOptions,
	)
	c.Check(err, gc.ErrorMatches, `instance provisioning failed \(blargh\)`)
}

type brokenAddresses struct {
	neverRefreshes
}

func (brokenAddresses) Addresses() ([]network.Address, error) {
	return nil, errors.Errorf("Addresses will never work")
}

func (s *BootstrapSuite) TestWaitSSHStopsOnBadError(c *gc.C) {
	ctx := cmdtesting.Context(c)
	_, err := common.WaitSSH(
		ctx.Stderr, nil, ssh.DefaultClient, "/bin/true", brokenAddresses{}, testSSHTimeout,
		common.DefaultHostSSHOptions,
	)
	c.Check(err, gc.ErrorMatches, "getting addresses: Addresses will never work")
	c.Check(cmdtesting.Stderr(ctx), gc.Equals, "Waiting for address\n")
}

type neverOpensPort struct {
	neverRefreshes
	addr string
}

func (n *neverOpensPort) Addresses() ([]network.Address, error) {
	return network.NewAddresses(n.addr), nil
}

func (s *BootstrapSuite) TestWaitSSHTimesOutWaitingForDial(c *gc.C) {
	ctx := cmdtesting.Context(c)
	// 0.x.y.z addresses are always invalid
	_, err := common.WaitSSH(
		ctx.Stderr, nil, ssh.DefaultClient, "/bin/true", &neverOpensPort{addr: "0.1.2.3"}, testSSHTimeout,
		common.DefaultHostSSHOptions,
	)
	c.Check(err, gc.ErrorMatches,
		`waited for `+testSSHTimeout.Timeout.String()+` without being able to connect: mock connection failure to 0.1.2.3`)
	c.Check(cmdtesting.Stderr(ctx), gc.Matches,
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
	ctx := cmdtesting.Context(c)
	timeout := testSSHTimeout
	timeout.Timeout = 1 * time.Minute
	interrupted := make(chan os.Signal, 1)
	_, err := common.WaitSSH(
		ctx.Stderr, interrupted, ssh.DefaultClient, "", &interruptOnDial{name: "0.1.2.3", interrupted: interrupted}, timeout,
		common.DefaultHostSSHOptions,
	)
	c.Check(err, gc.ErrorMatches, "interrupted")
	// Exact timing is imprecise but it should have tried a few times before being killed
	c.Check(cmdtesting.Stderr(ctx), gc.Matches,
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

func (ac *addressesChange) Status() instance.InstanceStatus {
	return instance.InstanceStatus{}
}

func (ac *addressesChange) Addresses() ([]network.Address, error) {
	return network.NewAddresses(ac.addrs[0]...), nil
}

func (s *BootstrapSuite) TestWaitSSHRefreshAddresses(c *gc.C) {
	coretesting.SkipIfWindowsBug(c, "lp:1604961")
	ctx := cmdtesting.Context(c)
	_, err := common.WaitSSH(ctx.Stderr, nil, ssh.DefaultClient, "", &addressesChange{addrs: [][]string{
		nil,
		nil,
		{"0.1.2.3"},
		{"0.1.2.3"},
		nil,
		{"0.1.2.4"},
	}}, testSSHTimeout, common.DefaultHostSSHOptions)
	// Not necessarily the last one in the list, due to scheduling.
	c.Check(err, gc.ErrorMatches,
		`waited for `+testSSHTimeout.Timeout.String()+` without being able to connect: mock connection failure to 0.1.2.[34]`)
	stderr := cmdtesting.Stderr(ctx)
	c.Check(stderr, gc.Matches,
		"Waiting for address\n"+
			"(.|\n)*(Attempting to connect to 0.1.2.3:22\n)+(.|\n)*")
	c.Check(stderr, gc.Matches,
		"Waiting for address\n"+
			"(.|\n)*(Attempting to connect to 0.1.2.4:22\n)+(.|\n)*")
}

type FormatHardwareSuite struct{}

var _ = gc.Suite(&FormatHardwareSuite{})

func (s *FormatHardwareSuite) check(c *gc.C, hw *instance.HardwareCharacteristics, expected string) {
	c.Check(common.FormatHardware(hw), gc.Equals, expected)
}

func (s *FormatHardwareSuite) TestNil(c *gc.C) {
	s.check(c, nil, "")
}

func (s *FormatHardwareSuite) TestFieldsNil(c *gc.C) {
	s.check(c, &instance.HardwareCharacteristics{}, "")
}

func (s *FormatHardwareSuite) TestArch(c *gc.C) {
	arch := ""
	s.check(c, &instance.HardwareCharacteristics{Arch: &arch}, "")
	arch = "amd64"
	s.check(c, &instance.HardwareCharacteristics{Arch: &arch}, "arch=amd64")
}

func (s *FormatHardwareSuite) TestCores(c *gc.C) {
	var cores uint64
	s.check(c, &instance.HardwareCharacteristics{CpuCores: &cores}, "")
	cores = 24
	s.check(c, &instance.HardwareCharacteristics{CpuCores: &cores}, "cores=24")
}

func (s *FormatHardwareSuite) TestMem(c *gc.C) {
	var mem uint64
	s.check(c, &instance.HardwareCharacteristics{Mem: &mem}, "")
	mem = 800
	s.check(c, &instance.HardwareCharacteristics{Mem: &mem}, "mem=800M")
	mem = 1024
	s.check(c, &instance.HardwareCharacteristics{Mem: &mem}, "mem=1G")
	mem = 2712
	s.check(c, &instance.HardwareCharacteristics{Mem: &mem}, "mem=2.6G")
}

func (s *FormatHardwareSuite) TestAll(c *gc.C) {
	arch := "ppc64"
	var cores uint64 = 2
	var mem uint64 = 123
	hw := &instance.HardwareCharacteristics{
		Arch:     &arch,
		CpuCores: &cores,
		Mem:      &mem,
	}
	s.check(c, hw, "arch=ppc64 mem=123M cores=2")
}
