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
	loggo.GetLogger("juju.provider.local").SetLogLevel(loggo.TRACE)
	s.PatchEnvironment("http-proxy", "")
	s.PatchEnvironment("HTTP-PROXY", "")
	s.PatchEnvironment("https-proxy", "")
	s.PatchEnvironment("HTTPS-PROXY", "")
	s.PatchEnvironment("ftp-proxy", "")
	s.PatchEnvironment("FTP-PROXY", "")
	s.HookCommandOutput(&utils.AptCommandOutput, nil, nil)
}

func (s *prepareSuite) TestPrepareCapturesEnvironment(c *gc.C) {
	c.Skip("fails if local provider running already")
	baseConfig, err := config.New(config.UseDefaults, map[string]interface{}{
		"type": provider.Local,
		"name": "test",
	})
	c.Assert(err, gc.IsNil)
	provider, err := environs.Provider(provider.Local)
	c.Assert(err, gc.IsNil)
	defer local.MockAddressForInterface()()

	for i, test := range []struct {
		message          string
		extraConfig      map[string]interface{}
		env              map[string]string
		aptOutput        string
		expectedProxy    osenv.ProxySettings
		expectedAptProxy osenv.ProxySettings
	}{{
		message: "nothing set",
	}, {
		message: "grabs proxy from environment",
		env: map[string]string{
			"http_proxy":  "http://user@10.0.0.1",
			"HTTPS_PROXY": "https://user@10.0.0.1",
			"ftp_proxy":   "ftp://user@10.0.0.1",
		},
		expectedProxy: osenv.ProxySettings{
			Http:  "http://user@10.0.0.1",
			Https: "https://user@10.0.0.1",
			Ftp:   "ftp://user@10.0.0.1",
		},
		expectedAptProxy: osenv.ProxySettings{
			Http:  "http://user@10.0.0.1",
			Https: "https://user@10.0.0.1",
			Ftp:   "ftp://user@10.0.0.1",
		},
	}, {
		message: "skips proxy from environment if http-proxy set",
		extraConfig: map[string]interface{}{
			"http-proxy": "http://user@10.0.0.42",
		},
		env: map[string]string{
			"http_proxy":  "http://user@10.0.0.1",
			"HTTPS_PROXY": "https://user@10.0.0.1",
			"ftp_proxy":   "ftp://user@10.0.0.1",
		},
		expectedProxy: osenv.ProxySettings{
			Http: "http://user@10.0.0.42",
		},
		expectedAptProxy: osenv.ProxySettings{
			Http: "http://user@10.0.0.42",
		},
	}, {
		message: "skips proxy from environment if https-proxy set",
		extraConfig: map[string]interface{}{
			"https-proxy": "https://user@10.0.0.42",
		},
		env: map[string]string{
			"http_proxy":  "http://user@10.0.0.1",
			"HTTPS_PROXY": "https://user@10.0.0.1",
			"ftp_proxy":   "ftp://user@10.0.0.1",
		},
		expectedProxy: osenv.ProxySettings{
			Https: "https://user@10.0.0.42",
		},
		expectedAptProxy: osenv.ProxySettings{
			Https: "https://user@10.0.0.42",
		},
	}, {
		message: "skips proxy from environment if ftp-proxy set",
		extraConfig: map[string]interface{}{
			"ftp-proxy": "ftp://user@10.0.0.42",
		},
		env: map[string]string{
			"http_proxy":  "http://user@10.0.0.1",
			"HTTPS_PROXY": "https://user@10.0.0.1",
			"ftp_proxy":   "ftp://user@10.0.0.1",
		},
		expectedProxy: osenv.ProxySettings{
			Ftp: "ftp://user@10.0.0.42",
		},
		expectedAptProxy: osenv.ProxySettings{
			Ftp: "ftp://user@10.0.0.42",
		},
	}, {
		message: "apt-proxies detected",
		aptOutput: `CommandLine::AsString "apt-config dump";
Acquire::http::Proxy  "10.0.3.1:3142";
Acquire::https::Proxy "false";
Acquire::ftp::Proxy "none";
Acquire::magic::Proxy "none";
`,
		expectedAptProxy: osenv.ProxySettings{
			Http:  "10.0.3.1:3142",
			Https: "false",
			Ftp:   "none",
		},
	}, {
		message: "apt-proxies not used if apt-http-proxy set",
		extraConfig: map[string]interface{}{
			"apt-http-proxy": "value-set",
		},
		aptOutput: `CommandLine::AsString "apt-config dump";
Acquire::http::Proxy  "10.0.3.1:3142";
Acquire::https::Proxy "false";
Acquire::ftp::Proxy "none";
Acquire::magic::Proxy "none";
`,
		expectedAptProxy: osenv.ProxySettings{
			Http: "value-set",
		},
	}, {
		message: "apt-proxies not used if apt-https-proxy set",
		extraConfig: map[string]interface{}{
			"apt-https-proxy": "value-set",
		},
		aptOutput: `CommandLine::AsString "apt-config dump";
Acquire::http::Proxy  "10.0.3.1:3142";
Acquire::https::Proxy "false";
Acquire::ftp::Proxy "none";
Acquire::magic::Proxy "none";
`,
		expectedAptProxy: osenv.ProxySettings{
			Https: "value-set",
		},
	}, {
		message: "apt-proxies not used if apt-ftp-proxy set",
		extraConfig: map[string]interface{}{
			"apt-ftp-proxy": "value-set",
		},
		aptOutput: `CommandLine::AsString "apt-config dump";
Acquire::http::Proxy  "10.0.3.1:3142";
Acquire::https::Proxy "false";
Acquire::ftp::Proxy "none";
Acquire::magic::Proxy "none";
`,
		expectedAptProxy: osenv.ProxySettings{
			Ftp: "value-set",
		},
	}} {
		c.Logf("\n%v: %s", i, test.message)
		cleanup := []func(){}
		for key, value := range test.env {
			restore := testbase.PatchEnvironment(key, value)
			cleanup = append(cleanup, restore)
		}
		_, restore := testbase.HookCommandOutput(&utils.AptCommandOutput, []byte(test.aptOutput), nil)
		cleanup = append(cleanup, restore)
		testConfig := baseConfig
		if test.extraConfig != nil {
			testConfig, err = baseConfig.Apply(test.extraConfig)
			c.Assert(err, gc.IsNil)
		}
		env, err := provider.Prepare(testConfig)
		c.Assert(err, gc.IsNil)

		envConfig := env.Config()
		c.Assert(envConfig.HttpProxy(), gc.Equals, test.expectedProxy.Http)
		c.Assert(envConfig.HttpsProxy(), gc.Equals, test.expectedProxy.Https)
		c.Assert(envConfig.FtpProxy(), gc.Equals, test.expectedProxy.Ftp)

		c.Assert(envConfig.AptHttpProxy(), gc.Equals, test.expectedAptProxy.Http)
		c.Assert(envConfig.AptHttpsProxy(), gc.Equals, test.expectedAptProxy.Https)
		c.Assert(envConfig.AptFtpProxy(), gc.Equals, test.expectedAptProxy.Ftp)

		for _, clean := range cleanup {
			clean()
		}
	}
}
