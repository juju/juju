// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local_test

import (
	gc "launchpad.net/gocheck"
	"launchpad.net/loggo"

	lxctesting "launchpad.net/juju-core/container/lxc/testing"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/provider"
	"launchpad.net/juju-core/provider/local"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/utils"
)

type baseProviderSuite struct {
	lxctesting.TestSuite
	home    *testing.FakeHome
	restore func()
}

func (s *baseProviderSuite) SetUpTest(c *gc.C) {
	s.TestSuite.SetUpTest(c)
	s.home = testing.MakeFakeHomeNoEnvironments(c, "test")
	loggo.GetLogger("juju.provider.local").SetLogLevel(loggo.TRACE)
	s.restore = local.MockAddressForInterface()
}

func (s *baseProviderSuite) TearDownTest(c *gc.C) {
	s.restore()
	s.home.Restore()
	s.TestSuite.TearDownTest(c)
}

type prepareSuite struct {
	testing.FakeHomeSuite
}

var _ = gc.Suite(&prepareSuite{})

func (s *prepareSuite) SetUpTest(c *gc.C) {
	s.FakeHomeSuite.SetUpTest(c)
	s.PatchEnvironment("http-proxy", "")
	s.PatchEnvironment("HTTP-PROXY", "")
	s.PatchEnvironment("https-proxy", "")
	s.PatchEnvironment("HTTPS-PROXY", "")
	s.PatchEnvironment("ftp-proxy", "")
	s.PatchEnvironment("FTP-PROXY", "")
	s.HookCommandOutput(&utils.AptCommandOutput, nil, nil)

}

func (s *prepareSuite) TestPrepareCapturesEnvironment(c *gc.C) {
	baseConfig, err := config.New(config.UseDefaults, map[string]interface{}{
		"type": provider.Local,
		"name": "test",
	})
	c.Assert(err, gc.IsNil)
	provider, err := environs.Provider(provider.Local)
	c.Assert(err, gc.IsNil)

	for i, test := range []struct {
		message          string
		env              map[string]string
		aptOutput        string
		expectedProxy    osenv.ProxySettings
		expectedAptProxy osenv.ProxySettings
	}{{
		message: "nothing set",
	}} {
		c.Logf("%v: %s", i, test.message)
		for key, value := range test.env {
			s.PatchEnvironment(key, value)
		}
		_, restore := testbase.HookCommandOutput(&utils.AptCommandOutput, []byte(test.aptOutput), nil)

		env, err := provider.Prepare(baseConfig)
		c.Assert(err, gc.IsNil)

		envConfig := env.Config()
		c.Assert(envConfig.HttpProxy(), gc.Equals, test.expectedProxy.Http)
		c.Assert(envConfig.HttpsProxy(), gc.Equals, test.expectedProxy.Https)
		c.Assert(envConfig.FtpProxy(), gc.Equals, test.expectedProxy.Ftp)

		restore()
	}
}
