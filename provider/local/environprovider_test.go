// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local_test

import (
	"errors"
	"fmt"
	"os/user"

	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/packaging/manager"
	"github.com/juju/utils/proxy"
	gc "gopkg.in/check.v1"

	lxctesting "github.com/juju/juju/container/lxc/testing"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider"
	"github.com/juju/juju/provider/local"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type baseProviderSuite struct {
	lxctesting.TestSuite
	restore func()
}

func (s *baseProviderSuite) SetUpTest(c *gc.C) {
	s.TestSuite.SetUpTest(c)
	loggo.GetLogger("juju.provider.local").SetLogLevel(loggo.TRACE)
	s.restore = local.MockAddressForInterface()
	s.PatchValue(&local.VerifyPrerequisites, func(containerType instance.ContainerType) error {
		return nil
	})
}

func (s *baseProviderSuite) TearDownTest(c *gc.C) {
	s.restore()
	s.TestSuite.TearDownTest(c)
}

type prepareSuite struct {
	coretesting.FakeJujuHomeSuite
}

var _ = gc.Suite(&prepareSuite{})

func (s *prepareSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	loggo.GetLogger("juju.provider.local").SetLogLevel(loggo.TRACE)
	s.PatchEnvironment("http_proxy", "")
	s.PatchEnvironment("HTTP_PROXY", "")
	s.PatchEnvironment("https_proxy", "")
	s.PatchEnvironment("HTTPS_PROXY", "")
	s.PatchEnvironment("ftp_proxy", "")
	s.PatchEnvironment("FTP_PROXY", "")
	s.PatchEnvironment("no_proxy", "")
	s.PatchEnvironment("NO_PROXY", "")
	s.HookCommandOutput(&manager.CommandOutput, nil, nil)
	s.PatchValue(local.CheckLocalPort, func(port int, desc string) error {
		return nil
	})
	restore := local.MockAddressForInterface()
	s.AddCleanup(func(*gc.C) { restore() })
}

func (s *prepareSuite) TestPrepareCapturesEnvironment(c *gc.C) {
	baseConfig, err := config.New(config.UseDefaults, map[string]interface{}{
		"type": provider.Local,
		"name": "test",
	})
	c.Assert(err, jc.ErrorIsNil)
	provider, err := environs.Provider(provider.Local)
	c.Assert(err, jc.ErrorIsNil)

	for i, test := range []struct {
		message          string
		extraConfig      map[string]interface{}
		env              map[string]string
		aptOutput        string
		expectedProxy    proxy.Settings
		expectedAptProxy proxy.Settings
	}{{
		message: "nothing set",
	}, {
		message: "grabs proxy from environment",
		env: map[string]string{
			"http_proxy":  "http://user@10.0.0.1",
			"HTTPS_PROXY": "https://user@10.0.0.1",
			"ftp_proxy":   "ftp://user@10.0.0.1",
			"no_proxy":    "localhost,10.0.3.1",
		},
		expectedProxy: proxy.Settings{
			Http:    "http://user@10.0.0.1",
			Https:   "https://user@10.0.0.1",
			Ftp:     "ftp://user@10.0.0.1",
			NoProxy: "localhost,10.0.3.1",
		},
		expectedAptProxy: proxy.Settings{
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
		expectedProxy: proxy.Settings{
			Http: "http://user@10.0.0.42",
		},
		expectedAptProxy: proxy.Settings{
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
		expectedProxy: proxy.Settings{
			Https: "https://user@10.0.0.42",
		},
		expectedAptProxy: proxy.Settings{
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
		expectedProxy: proxy.Settings{
			Ftp: "ftp://user@10.0.0.42",
		},
		expectedAptProxy: proxy.Settings{
			Ftp: "ftp://user@10.0.0.42",
		},
	}, {
		message: "skips proxy from environment if no-proxy set",
		extraConfig: map[string]interface{}{
			"no-proxy": "localhost,10.0.3.1",
		},
		env: map[string]string{
			"http_proxy":  "http://user@10.0.0.1",
			"HTTPS_PROXY": "https://user@10.0.0.1",
			"ftp_proxy":   "ftp://user@10.0.0.1",
		},
		expectedProxy: proxy.Settings{
			NoProxy: "localhost,10.0.3.1",
		},
	}, {
		message: "apt-proxies detected",
		aptOutput: `CommandLine::AsString "apt-config dump";
Acquire::http::Proxy  "10.0.3.1:3142";
Acquire::https::Proxy "";
Acquire::ftp::Proxy "";
Acquire::magic::Proxy "";
`,
		expectedAptProxy: proxy.Settings{
			Http:  "http://10.0.3.1:3142",
			Https: "",
			Ftp:   "",
		},
	}, {
		message: "apt-proxies not used if apt-http-proxy set",
		extraConfig: map[string]interface{}{
			"apt-http-proxy": "http://value-set",
		},
		aptOutput: `CommandLine::AsString "apt-config dump";
Acquire::http::Proxy  "10.0.3.1:3142";
Acquire::https::Proxy "";
Acquire::ftp::Proxy "";
Acquire::magic::Proxy "";
`,
		expectedAptProxy: proxy.Settings{
			Http: "http://value-set",
		},
	}, {
		message: "apt-proxies not used if apt-https-proxy set",
		extraConfig: map[string]interface{}{
			"apt-https-proxy": "https://value-set",
		},
		aptOutput: `CommandLine::AsString "apt-config dump";
Acquire::http::Proxy  "10.0.3.1:3142";
Acquire::https::Proxy "";
Acquire::ftp::Proxy "";
Acquire::magic::Proxy "";
`,
		expectedAptProxy: proxy.Settings{
			Https: "https://value-set",
		},
	}, {
		message: "apt-proxies not used if apt-ftp-proxy set",
		extraConfig: map[string]interface{}{
			"apt-ftp-proxy": "ftp://value-set",
		},
		aptOutput: `CommandLine::AsString "apt-config dump";
Acquire::http::Proxy  "10.0.3.1:3142";
Acquire::https::Proxy "";
Acquire::ftp::Proxy "";
Acquire::magic::Proxy "";
`,
		expectedAptProxy: proxy.Settings{
			Ftp: "ftp://value-set",
		},
	}} {
		c.Logf("\n%v: %s", i, test.message)
		cleanup := []func(){}
		for key, value := range test.env {
			restore := testing.PatchEnvironment(key, value)
			cleanup = append(cleanup, restore)
		}
		_, restore := testing.HookCommandOutput(&manager.CommandOutput, []byte(test.aptOutput), nil)
		cleanup = append(cleanup, restore)
		testConfig := baseConfig
		if test.extraConfig != nil {
			testConfig, err = baseConfig.Apply(test.extraConfig)
			c.Assert(err, jc.ErrorIsNil)
		}
		env, err := provider.PrepareForBootstrap(envtesting.BootstrapContext(c), testConfig)
		c.Assert(err, jc.ErrorIsNil)

		envConfig := env.Config()
		c.Assert(envConfig.HttpProxy(), gc.Equals, test.expectedProxy.Http)
		c.Assert(envConfig.HttpsProxy(), gc.Equals, test.expectedProxy.Https)
		c.Assert(envConfig.FtpProxy(), gc.Equals, test.expectedProxy.Ftp)
		c.Assert(envConfig.NoProxy(), gc.Equals, test.expectedProxy.NoProxy)

		if version.Current.OS == version.Ubuntu {
			c.Assert(envConfig.AptHttpProxy(), gc.Equals, test.expectedAptProxy.Http)
			c.Assert(envConfig.AptHttpsProxy(), gc.Equals, test.expectedAptProxy.Https)
			c.Assert(envConfig.AptFtpProxy(), gc.Equals, test.expectedAptProxy.Ftp)
		}
		for _, clean := range cleanup {
			clean()
		}
	}
}

func (s *prepareSuite) TestPrepareNamespace(c *gc.C) {
	s.PatchValue(local.DetectPackageProxies, func() (proxy.Settings, error) {
		return proxy.Settings{}, nil
	})
	basecfg, err := config.New(config.UseDefaults, map[string]interface{}{
		"type": "local",
		"name": "test",
	})
	provider, err := environs.Provider("local")
	c.Assert(err, jc.ErrorIsNil)

	type test struct {
		userEnv   string
		userOS    string
		userOSErr error
		namespace string
		err       string
	}
	tests := []test{{
		userEnv:   "someone",
		userOS:    "other",
		namespace: "someone-test",
	}, {
		userOS:    "other",
		namespace: "other-test",
	}, {
		userOSErr: errors.New("oh noes"),
		err:       "failed to determine username for namespace: oh noes",
	}}

	for i, test := range tests {
		c.Logf("test %d: %v", i, test)
		s.PatchEnvironment("USER", test.userEnv)
		s.PatchValue(local.UserCurrent, func() (*user.User, error) {
			return &user.User{Username: test.userOS}, test.userOSErr
		})
		env, err := provider.PrepareForBootstrap(envtesting.BootstrapContext(c), basecfg)
		if test.err == "" {
			c.Assert(err, jc.ErrorIsNil)
			cfg := env.Config()
			c.Assert(cfg.UnknownAttrs()["namespace"], gc.Equals, test.namespace)
		} else {
			c.Assert(err, gc.ErrorMatches, test.err)
		}
	}
}

func (s *prepareSuite) TestPrepareProxySSH(c *gc.C) {
	s.PatchValue(local.DetectPackageProxies, func() (proxy.Settings, error) {
		return proxy.Settings{}, nil
	})
	basecfg, err := config.New(config.UseDefaults, map[string]interface{}{
		"type": "local",
		"name": "test",
	})
	provider, err := environs.Provider("local")
	c.Assert(err, jc.ErrorIsNil)
	env, err := provider.PrepareForBootstrap(envtesting.BootstrapContext(c), basecfg)
	c.Assert(err, jc.ErrorIsNil)
	// local provider sets proxy-ssh to false
	c.Assert(env.Config().ProxySSH(), jc.IsFalse)
}

func (s *prepareSuite) TestProxyLocalhostFix(c *gc.C) {
	basecfg, err := config.New(config.UseDefaults, map[string]interface{}{
		"type": "local",
		"name": "test",
	})
	c.Assert(err, jc.ErrorIsNil)
	provider, err := environs.Provider(provider.Local)
	c.Assert(err, jc.ErrorIsNil)

	//URL protocol is irrelelvant as we are only interested in it as string
	urlConstruct := "http://%v%v"
	// This value is currently hard-coded in export_test and is called on by this test setup. @see export_test.go MockAddressForInterface()
	expectedBridge := "127.0.0.1"
	for i, test := range urlReplacementTests {
		c.Logf("test %d: %v\n", i, test.message)

		//construct proxy env attributes based on the test scenario
		proxyAttrValues := map[string]interface{}{}
		for _, anAttrKey := range config.ProxyAttributes {
			proxyAttrValues[anAttrKey] = fmt.Sprintf(urlConstruct, test.url, test.port)
		}

		//Update env config to include new attributes
		cfg, err := basecfg.Apply(proxyAttrValues)
		c.Assert(err, jc.ErrorIsNil)
		//this call should replace all loopback urls with bridge ip
		env, err := provider.PrepareForBootstrap(envtesting.BootstrapContext(c), cfg)
		c.Assert(err, jc.ErrorIsNil)

		// verify that correct replacement took place
		envConfig := env.Config().AllAttrs()
		for _, anAttrKey := range config.ProxyAttributes {
			//expected value is either unchanged original
			expectedAttValue := proxyAttrValues[anAttrKey]
			if test.expectChange {
				// or expected value has bridge ip substituted for localhost variations
				expectedAttValue = fmt.Sprintf(urlConstruct, expectedBridge, test.port)
			}
			c.Assert(envConfig[anAttrKey].(string), gc.Equals, expectedAttValue.(string))
		}
	}
}

type testURL struct {
	message      string
	url          string
	port         string
	expectChange bool
}

var urlReplacementTests = []testURL{{
	message:      "replace localhost with bridge ip in proxy url",
	url:          "localhost",
	port:         "",
	expectChange: true,
}, {
	message:      "replace localhost:port with bridge ip:port in proxy url",
	url:          "localhost",
	port:         ":8877",
	expectChange: true,
}, {
	message:      "replace 127.2.0.1 with bridge ip in proxy url",
	url:          "127.2.0.1",
	port:         "",
	expectChange: true,
}, {
	message:      "replace 127.2.0.1:port with bridge ip:port in proxy url",
	url:          "127.2.0.1",
	port:         ":8877",
	expectChange: true,
}, {
	message:      "replace [::1]:port with bridge ip:port in proxy url",
	url:          "[::1]",
	port:         ":8877",
	expectChange: true,
}, {
	// Note that http//::1 (without the square brackets)
	// is not a legal URL. See https://www.ietf.org/rfc/rfc2732.txt.
	message:      "replace [::1] with bridge ip in proxy url",
	url:          "[::1]",
	port:         "",
	expectChange: true,
}, {
	message:      "do not replace provided with bridge ip in proxy url",
	url:          "www.google.com",
	port:         "",
	expectChange: false,
}, {
	message:      "do not replace provided:port with bridge ip:port in proxy url",
	url:          "www.google.com",
	port:         ":8877",
	expectChange: false,
}, {
	message:      "lp 1437296 - apt-http-proxy being reset to bridge address when shouldn't",
	url:          "192.168.1.201",
	port:         ":8000",
	expectChange: false,
},
}
