// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	stdtesting "testing"

	"github.com/juju/gomaasapi"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/errors"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/provider/maas"
	coretesting "github.com/juju/juju/testing"
)

type environSuite struct {
	coretesting.BaseSuite
	envtesting.ToolsFixture
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
}

func (s *environSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)

	mockCapabilities := func(*gomaasapi.MAASObject, string) (set.Strings, error) {
		return set.NewStrings("network-deployment-ubuntu"), nil
	}
	mockGetController := func(string, string) (gomaasapi.Controller, error) {
		return nil, gomaasapi.NewUnsupportedVersionError("oops")
	}
	s.PatchValue(&maas.GetCapabilities, mockCapabilities)
	s.PatchValue(&maas.GetMAAS2Controller, mockGetController)
}

func (s *environSuite) TearDownTest(c *gc.C) {
	s.ToolsFixture.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

func (s *environSuite) TearDownSuite(c *gc.C) {
	s.restoreTimeouts()
	s.BaseSuite.TearDownSuite(c)
}

func getSimpleTestConfig(c *gc.C, extraAttrs coretesting.Attrs) *config.Config {
	attrs := coretesting.FakeConfig()
	attrs["type"] = "maas"
	attrs["bootstrap-timeout"] = "1200"
	for k, v := range extraAttrs {
		attrs[k] = v
	}
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

func getSimpleCloudSpec() environs.CloudSpec {
	cred := cloud.NewCredential(cloud.OAuth1AuthType, map[string]string{
		"maas-oauth": "a:b:c",
	})
	return environs.CloudSpec{
		Type:       "maas",
		Name:       "maas",
		Endpoint:   "http://maas.testing.invalid",
		Credential: &cred,
	}
}

func (*environSuite) TestSetConfigValidatesFirst(c *gc.C) {
	// SetConfig() validates the config change and disallows, for example,
	// changes in the environment name.
	oldCfg := getSimpleTestConfig(c, coretesting.Attrs{"name": "old-name"})
	newCfg := getSimpleTestConfig(c, coretesting.Attrs{"name": "new-name"})
	env, err := maas.NewEnviron(getSimpleCloudSpec(), oldCfg)
	c.Assert(err, jc.ErrorIsNil)

	// SetConfig() fails, even though both the old and the new config are
	// individually valid.
	err = env.SetConfig(newCfg)
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, ".*cannot change name.*")

	// The old config is still in place.  The new config never took effect.
	c.Check(env.Config().Name(), gc.Equals, "old-name")
}

func (*environSuite) TestSetConfigUpdatesConfig(c *gc.C) {
	origAttrs := coretesting.Attrs{
		"apt-mirror": "http://testing1.invalid",
	}
	cfg := getSimpleTestConfig(c, origAttrs)
	env, err := maas.NewEnviron(getSimpleCloudSpec(), cfg)
	c.Check(err, jc.ErrorIsNil)
	c.Check(env.Config().Name(), gc.Equals, "testenv")

	newAttrs := coretesting.Attrs{
		"apt-mirror": "http://testing2.invalid",
	}
	cfg2 := getSimpleTestConfig(c, newAttrs)
	errSetConfig := env.SetConfig(cfg2)
	c.Check(errSetConfig, gc.IsNil)
	c.Check(env.Config().Name(), gc.Equals, "testenv")
	c.Check(env.Config().AptMirror(), gc.Equals, "http://testing2.invalid")
}

func (*environSuite) TestNewEnvironSetsConfig(c *gc.C) {
	cfg := getSimpleTestConfig(c, nil)

	env, err := maas.NewEnviron(getSimpleCloudSpec(), cfg)

	c.Check(err, jc.ErrorIsNil)
	c.Check(env.Config().Name(), gc.Equals, "testenv")
}

var expectedCloudinitConfig = []string{
	"set -xe",
	"mkdir -p '/var/lib/juju'\ncat > '/var/lib/juju/MAASmachine.txt' << 'EOF'\n'hostname: testing.invalid\n'\nEOF\nchmod 0755 '/var/lib/juju/MAASmachine.txt'",
}

func (*environSuite) TestNewCloudinitConfig(c *gc.C) {
	cfg := getSimpleTestConfig(c, nil)
	env, err := maas.NewEnviron(getSimpleCloudSpec(), cfg)
	c.Assert(err, jc.ErrorIsNil)
	modifyNetworkScript := maas.RenderEtcNetworkInterfacesScript("eth0", "eth1")
	script := expectedCloudinitConfig
	script = append(script, modifyNetworkScript)
	cloudcfg, err := maas.NewCloudinitConfig(env, "testing.invalid", "quantal", []string{"eth0", "eth1"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cloudcfg.SystemUpdate(), jc.IsTrue)
	c.Assert(cloudcfg.RunCmds(), jc.DeepEquals, script)
}

func (*environSuite) TestNewCloudinitConfigWithDisabledNetworkManagement(c *gc.C) {
	attrs := coretesting.Attrs{
		"disable-network-management": true,
	}
	cfg := getSimpleTestConfig(c, attrs)
	env, err := maas.NewEnviron(getSimpleCloudSpec(), cfg)
	c.Assert(err, jc.ErrorIsNil)
	cloudcfg, err := maas.NewCloudinitConfig(env, "testing.invalid", "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cloudcfg.SystemUpdate(), jc.IsTrue)
	c.Assert(cloudcfg.RunCmds(), jc.DeepEquals, expectedCloudinitConfig)
}

func (*environSuite) TestRenderEtcNetworkInterfacesScriptMultipleNames(c *gc.C) {
	script := maas.RenderEtcNetworkInterfacesScript("eth0", "eth0:1", "eth2", "eth1")
	c.Check(script, jc.Contains, `--interfaces-to-bridge="eth0 eth0:1 eth2 eth1"`)
	c.Check(script, jc.Contains, `--bridge-prefix="br-"`)
}

func (*environSuite) TestRenderEtcNetworkInterfacesScriptSingleName(c *gc.C) {
	script := maas.RenderEtcNetworkInterfacesScript("eth0")
	c.Check(script, jc.Contains, `--interfaces-to-bridge="eth0"`)
	c.Check(script, jc.Contains, `--bridge-prefix="br-"`)
}

type badEndpointSuite struct {
	coretesting.BaseSuite

	fakeServer *httptest.Server
	cloudSpec  environs.CloudSpec
}

var _ = gc.Suite(&badEndpointSuite{})

func (s *badEndpointSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	always404 := func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, "uh-oh")
	}
	s.fakeServer = httptest.NewServer(http.HandlerFunc(always404))
}

func (s *badEndpointSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	cred := cloud.NewCredential(cloud.OAuth1AuthType, map[string]string{
		"maas-oauth": "a:b:c",
	})
	s.cloudSpec = environs.CloudSpec{
		Type:       "maas",
		Name:       "maas",
		Endpoint:   s.fakeServer.URL,
		Credential: &cred,
	}
}

func (s *badEndpointSuite) TestBadEndpointMessageNoMAAS(c *gc.C) {
	cfg := getSimpleTestConfig(c, coretesting.Attrs{})
	env, err := maas.NewEnviron(s.cloudSpec, cfg)
	c.Assert(env, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `could not connect to MAAS controller - check the endpoint is correct \(it normally ends with /MAAS\)`)
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
}

func (s *badEndpointSuite) TestBadEndpointMessageWithMAAS(c *gc.C) {
	cfg := getSimpleTestConfig(c, coretesting.Attrs{})
	s.cloudSpec.Endpoint += "/MAAS"
	env, err := maas.NewEnviron(s.cloudSpec, cfg)
	c.Assert(env, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `could not connect to MAAS controller - check the endpoint is correct`)
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
}

func (s *badEndpointSuite) TestBadEndpointMessageWithMAASAndSlash(c *gc.C) {
	cfg := getSimpleTestConfig(c, coretesting.Attrs{})
	s.cloudSpec.Endpoint += "/MAAS/"
	env, err := maas.NewEnviron(s.cloudSpec, cfg)
	c.Assert(env, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `could not connect to MAAS controller - check the endpoint is correct`)
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
}
