// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas_test

import (
	stdtesting "testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"launchpad.net/gomaasapi"

	"github.com/juju/juju/environs/config"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/provider/maas"
	coretesting "github.com/juju/juju/testing"
)

type environSuite struct {
	coretesting.BaseSuite
	envtesting.ToolsFixture
	testMAASObject  *gomaasapi.TestMAASObject
	restoreTimeouts func()
}

var _ = gc.Suite(&environSuite{})

func TestMAAS(t *stdtesting.T) {
	gc.TestingT(t)
}

// TDOO: jam 2013-12-06 This is copied from the providerSuite which is in a
// whitebox package maas. Either move that into a whitebox test so it can be
// shared, or into a 'testing' package so we can use it here.
func (s *environSuite) SetUpSuite(c *gc.C) {
	s.restoreTimeouts = envtesting.PatchAttemptStrategies(maas.ShortAttempt)
	s.BaseSuite.SetUpSuite(c)
	TestMAASObject := gomaasapi.NewTestMAAS("1.0")
	s.testMAASObject = TestMAASObject
}

func (s *environSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
	s.SetFeatureFlags(feature.AddressAllocation)
}

func (s *environSuite) TearDownTest(c *gc.C) {
	s.testMAASObject.TestServer.Clear()
	s.ToolsFixture.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

func (s *environSuite) TearDownSuite(c *gc.C) {
	s.testMAASObject.Close()
	s.restoreTimeouts()
	s.BaseSuite.TearDownSuite(c)
}

func getSimpleTestConfig(c *gc.C, extraAttrs coretesting.Attrs) *config.Config {
	attrs := coretesting.FakeConfig()
	attrs["type"] = "maas"
	attrs["maas-server"] = "http://maas.testing.invalid"
	attrs["maas-oauth"] = "a:b:c"
	for k, v := range extraAttrs {
		attrs[k] = v
	}
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

func (*environSuite) TestSetConfigValidatesFirst(c *gc.C) {
	// SetConfig() validates the config change and disallows, for example,
	// changes in the environment name.
	oldCfg := getSimpleTestConfig(c, coretesting.Attrs{"name": "old-name"})
	newCfg := getSimpleTestConfig(c, coretesting.Attrs{"name": "new-name"})
	env, err := maas.NewEnviron(oldCfg)
	c.Assert(err, jc.ErrorIsNil)

	// SetConfig() fails, even though both the old and the new config are
	// individually valid.
	err = env.SetConfig(newCfg)
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, ".*cannot change name.*")

	// The old config is still in place.  The new config never took effect.
	c.Check(env.Config().Name(), gc.Equals, "old-name")
}

func (*environSuite) TestSetConfigRefusesChangingAgentName(c *gc.C) {
	oldCfg := getSimpleTestConfig(c, coretesting.Attrs{"maas-agent-name": "agent-one"})
	newCfgTwo := getSimpleTestConfig(c, coretesting.Attrs{"maas-agent-name": "agent-two"})
	env, err := maas.NewEnviron(oldCfg)
	c.Assert(err, jc.ErrorIsNil)

	// SetConfig() fails, even though both the old and the new config are
	// individually valid.
	err = env.SetConfig(newCfgTwo)
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, ".*cannot change maas-agent-name.*")

	// The old config is still in place.  The new config never took effect.
	c.Check(maas.MAASAgentName(env), gc.Equals, "agent-one")

	// It also refuses to set it to the empty string:
	err = env.SetConfig(getSimpleTestConfig(c, coretesting.Attrs{"maas-agent-name": ""}))
	c.Check(err, gc.ErrorMatches, ".*cannot change maas-agent-name.*")

	// And to nil
	err = env.SetConfig(getSimpleTestConfig(c, nil))
	c.Check(err, gc.ErrorMatches, ".*cannot change maas-agent-name.*")
}

func (*environSuite) TestSetConfigAllowsEmptyFromNilAgentName(c *gc.C) {
	// bug #1256179 is that when using an older version of Juju (<1.16.2)
	// we didn't include maas-agent-name in the database, so it was 'nil'
	// in the OldConfig. However, when setting an environment, we would set
	// it to "" (because maasEnvironConfig.Validate ensures it is a 'valid'
	// string). We can't create that from NewEnviron or newConfig because
	// both of them Validate the contents. 'cmd/juju/environment
	// SetEnvironmentCommand' instead uses conn.State.EnvironConfig() which
	// just reads the content of the database into a map, so we just create
	// the map ourselves.

	// Even though we use 'nil' here, it actually stores it as "" because
	// 1.16.2 already validates the value
	baseCfg := getSimpleTestConfig(c, coretesting.Attrs{"maas-agent-name": ""})
	c.Check(baseCfg.UnknownAttrs()["maas-agent-name"], gc.Equals, "")
	env, err := maas.NewEnviron(baseCfg)
	c.Assert(err, jc.ErrorIsNil)
	provider := env.Provider()

	attrs := coretesting.FakeConfig()
	// These are attrs we need to make it a valid Config, but would usually
	// be set by other infrastructure
	attrs["type"] = "maas"
	nilCfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	validatedConfig, err := provider.Validate(baseCfg, nilCfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(validatedConfig.UnknownAttrs()["maas-agent-name"], gc.Equals, "")
	// However, you can't set it to an actual value if you haven't been using a value
	valueCfg := getSimpleTestConfig(c, coretesting.Attrs{"maas-agent-name": "agent-name"})
	_, err = provider.Validate(valueCfg, nilCfg)
	c.Check(err, gc.ErrorMatches, ".*cannot change maas-agent-name.*")
}

func (*environSuite) TestSetConfigAllowsChangingNilAgentNameToEmptyString(c *gc.C) {
	oldCfg := getSimpleTestConfig(c, nil)
	newCfgTwo := getSimpleTestConfig(c, coretesting.Attrs{"maas-agent-name": ""})
	env, err := maas.NewEnviron(oldCfg)
	c.Assert(err, jc.ErrorIsNil)

	err = env.SetConfig(newCfgTwo)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(maas.MAASAgentName(env), gc.Equals, "")
}

func (*environSuite) TestSetConfigUpdatesConfig(c *gc.C) {
	origAttrs := coretesting.Attrs{
		"server-name":  "http://maas2.testing.invalid",
		"maas-oauth":   "a:b:c",
		"admin-secret": "secret",
	}
	cfg := getSimpleTestConfig(c, origAttrs)
	env, err := maas.NewEnviron(cfg)
	c.Check(err, jc.ErrorIsNil)
	c.Check(env.Config().Name(), gc.Equals, "testenv")

	anotherServer := "http://maas.testing.invalid"
	anotherOauth := "c:d:e"
	anotherSecret := "secret2"
	newAttrs := coretesting.Attrs{
		"server-name":  anotherServer,
		"maas-oauth":   anotherOauth,
		"admin-secret": anotherSecret,
	}
	cfg2 := getSimpleTestConfig(c, newAttrs)
	errSetConfig := env.SetConfig(cfg2)
	c.Check(errSetConfig, gc.IsNil)
	c.Check(env.Config().Name(), gc.Equals, "testenv")
	authClient, _ := gomaasapi.NewAuthenticatedClient(anotherServer, anotherOauth, maas.APIVersion)
	maasClient := gomaasapi.NewMAAS(*authClient)
	MAASServer := maas.GetMAASClient(env)
	c.Check(MAASServer, gc.DeepEquals, maasClient)
}

func (*environSuite) TestNewEnvironSetsConfig(c *gc.C) {
	cfg := getSimpleTestConfig(c, nil)

	env, err := maas.NewEnviron(cfg)

	c.Check(err, jc.ErrorIsNil)
	c.Check(env.Config().Name(), gc.Equals, "testenv")
}

var expectedCloudinitConfig = []string{
	"set -xe",
	"mkdir -p '/var/lib/juju'\ncat > '/var/lib/juju/MAASmachine.txt' << 'EOF'\n'hostname: testing.invalid\n'\nEOF\nchmod 0755 '/var/lib/juju/MAASmachine.txt'",
}

var expectedCloudinitConfigWithBridge = []string{
	"set -xe",
	"mkdir -p '/var/lib/juju'\ncat > '/var/lib/juju/MAASmachine.txt' << 'EOF'\n'hostname: testing.invalid\n'\nEOF\nchmod 0755 '/var/lib/juju/MAASmachine.txt'",
	"\n# In case we already created the bridge, don't do it again.\ngrep -q \"iface juju-br0 inet dhcp\" && exit 0\n\n# Discover primary interface at run-time using the default route (if set)\nPRIMARY_IFACE=$(ip route list exact 0/0 | egrep -o 'dev [^ ]+' | cut -b5-)\n\n# If $PRIMARY_IFACE is empty, there's nothing to do.\n[ -z \"$PRIMARY_IFACE\" ] && exit 0\n\n# Change the config to make $PRIMARY_IFACE manual instead of DHCP,\n# then create the bridge and enslave $PRIMARY_IFACE into it.\ngrep -q \"iface ${PRIMARY_IFACE} inet dhcp\" /etc/network/interfaces && \\\nsed -i \"s/iface ${PRIMARY_IFACE} inet dhcp//\" /etc/network/interfaces && \\\ncat >> /etc/network/interfaces << EOF\n\n# Primary interface (defining the default route)\niface ${PRIMARY_IFACE} inet manual\n\n# Bridge to use for LXC/KVM containers\nauto juju-br0\niface juju-br0 inet dhcp\n    bridge_ports ${PRIMARY_IFACE}\nEOF\n\n# Make the primary interface not auto-starting.\ngrep -q \"auto ${PRIMARY_IFACE}\" /etc/network/interfaces && \\\nsed -i \"s/auto ${PRIMARY_IFACE}//\" /etc/network/interfaces\n\n# Stop $PRIMARY_IFACE and start the bridge instead.\nifdown -v ${PRIMARY_IFACE} ; ifup -v juju-br0\n\n# Finally, remove the route using $PRIMARY_IFACE (if any) so it won't\n# clash with the same automatically added route for juju-br0 (except\n# for the device name).\nip route flush dev $PRIMARY_IFACE scope link proto kernel || true\n",
}

func (*environSuite) TestNewCloudinitConfigWithFeatureFlag(c *gc.C) {
	cfg := getSimpleTestConfig(c, nil)
	env, err := maas.NewEnviron(cfg)
	c.Assert(err, jc.ErrorIsNil)
	cloudcfg, err := maas.NewCloudinitConfig(env, "testing.invalid", "eth0", "quantal")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cloudcfg.SystemUpdate(), jc.IsTrue)
	c.Assert(cloudcfg.RunCmds(), jc.DeepEquals, expectedCloudinitConfig)
}

func (s *environSuite) TestNewCloudinitConfigNoFeatureFlag(c *gc.C) {
	cfg := getSimpleTestConfig(c, nil)
	env, err := maas.NewEnviron(cfg)
	c.Assert(err, jc.ErrorIsNil)
	testCase := func(expectedConfig []string) {
		cloudcfg, err := maas.NewCloudinitConfig(env, "testing.invalid", "eth0", "quantal")
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(cloudcfg.SystemUpdate(), jc.IsTrue)
		c.Assert(cloudcfg.RunCmds(), jc.DeepEquals, expectedConfig)
	}
	// First test the default case (address allocation feature flag on).
	testCase(expectedCloudinitConfig)

	// Now test with the flag off.
	s.SetFeatureFlags() // clear the flags.
	testCase(expectedCloudinitConfigWithBridge)
}

func (*environSuite) TestNewCloudinitConfigWithDisabledNetworkManagement(c *gc.C) {
	attrs := coretesting.Attrs{
		"disable-network-management": true,
	}
	cfg := getSimpleTestConfig(c, attrs)
	env, err := maas.NewEnviron(cfg)
	c.Assert(err, jc.ErrorIsNil)
	cloudcfg, err := maas.NewCloudinitConfig(env, "testing.invalid", "eth0", "quantal")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cloudcfg.SystemUpdate(), jc.IsTrue)
	c.Assert(cloudcfg.RunCmds(), jc.DeepEquals, expectedCloudinitConfig)
}
