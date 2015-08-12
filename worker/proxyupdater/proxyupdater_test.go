// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater_test

import (
	"io/ioutil"
	"os"
	"path"
	"runtime"
	"strings"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/packaging/commands"
	pacconfig "github.com/juju/utils/packaging/config"
	"github.com/juju/utils/proxy"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/environment"
	"github.com/juju/juju/environs/config"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/proxyupdater"
)

type ProxyUpdaterSuite struct {
	jujutesting.JujuConnSuite

	apiRoot        api.Connection
	environmentAPI *environment.Facade
	machine        *state.Machine

	proxyFile string
	started   chan struct{}
}

var _ = gc.Suite(&ProxyUpdaterSuite{})

func (s *ProxyUpdaterSuite) setStarted() {
	select {
	case <-s.started:
	default:
		close(s.started)
	}
}

func (s *ProxyUpdaterSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.apiRoot, s.machine = s.OpenAPIAsNewMachine(c)
	// Create the environment API facade.
	s.environmentAPI = s.apiRoot.Environment()
	c.Assert(s.environmentAPI, gc.NotNil)

	proxyDir := c.MkDir()
	s.PatchValue(&proxyupdater.ProxyDirectory, proxyDir)
	s.started = make(chan struct{})
	s.PatchValue(&proxyupdater.Started, s.setStarted)
	s.PatchValue(&pacconfig.AptProxyConfigFile, path.Join(proxyDir, "juju-apt-proxy"))
	s.proxyFile = path.Join(proxyDir, proxyupdater.ProxyFile)
}

func (s *ProxyUpdaterSuite) waitForPostSetup(c *gc.C) {
	select {
	case <-time.After(testing.LongWait):
		c.Fatalf("timeout while waiting for setup")
	case <-s.started:
	}
}

func (s *ProxyUpdaterSuite) waitProxySettings(c *gc.C, expected proxy.Settings) {
	for {
		select {
		case <-time.After(testing.LongWait):
			c.Fatalf("timeout while waiting for proxy settings to change")
		case <-time.After(10 * time.Millisecond):
			obtained := proxy.DetectProxies()
			if obtained != expected {
				c.Logf("proxy settings are %#v, still waiting", obtained)
				continue
			}
			return
		}
	}
}

func (s *ProxyUpdaterSuite) waitForFile(c *gc.C, filename, expected string) {
	//TODO(bogdanteleaga): Find a way to test this on windows
	if runtime.GOOS == "windows" {
		c.Skip("Proxy settings are written to the registry on windows")
	}
	for {
		select {
		case <-time.After(testing.LongWait):
			c.Fatalf("timeout while waiting for proxy settings to change")
		case <-time.After(10 * time.Millisecond):
			fileContent, err := ioutil.ReadFile(filename)
			if os.IsNotExist(err) {
				continue
			}
			c.Assert(err, jc.ErrorIsNil)
			if string(fileContent) != expected {
				c.Logf("file content not matching, still waiting")
				continue
			}
			return
		}
	}
}

func (s *ProxyUpdaterSuite) TestRunStop(c *gc.C) {
	updater := proxyupdater.New(s.environmentAPI, false)
	c.Assert(worker.Stop(updater), gc.IsNil)
}

func (s *ProxyUpdaterSuite) updateConfig(c *gc.C) (proxy.Settings, proxy.Settings) {

	proxySettings := proxy.Settings{
		Http:    "http proxy",
		Https:   "https proxy",
		Ftp:     "ftp proxy",
		NoProxy: "no proxy",
	}
	attrs := map[string]interface{}{}
	for k, v := range config.ProxyConfigMap(proxySettings) {
		attrs[k] = v
	}

	// We explicitly set apt proxy settings as well to show that it is the apt
	// settings that are used for the apt config, and not just the normal
	// proxy settings which is what we would get if we don't explicitly set
	// apt values.
	aptProxySettings := proxy.Settings{
		Http:  "http://apt.http.proxy",
		Https: "https://apt.https.proxy",
		Ftp:   "ftp://apt.ftp.proxy",
	}
	for k, v := range config.AptProxyConfigMap(aptProxySettings) {
		attrs[k] = v
	}

	err := s.State.UpdateEnvironConfig(attrs, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	return proxySettings, aptProxySettings
}

func (s *ProxyUpdaterSuite) TestInitialState(c *gc.C) {
	proxySettings, aptProxySettings := s.updateConfig(c)

	updater := proxyupdater.New(s.environmentAPI, true)
	defer worker.Stop(updater)

	s.waitProxySettings(c, proxySettings)
	s.waitForFile(c, s.proxyFile, proxySettings.AsScriptEnvironment()+"\n")

	paccmder, err := commands.NewPackageCommander(version.Current.Series)
	c.Assert(err, jc.ErrorIsNil)
	s.waitForFile(c, pacconfig.AptProxyConfigFile, paccmder.ProxyConfigContents(aptProxySettings)+"\n")
}

func (s *ProxyUpdaterSuite) TestWriteSystemFiles(c *gc.C) {
	proxySettings, aptProxySettings := s.updateConfig(c)

	updater := proxyupdater.New(s.environmentAPI, true)
	defer worker.Stop(updater)
	s.waitForPostSetup(c)

	s.waitProxySettings(c, proxySettings)
	s.waitForFile(c, s.proxyFile, proxySettings.AsScriptEnvironment()+"\n")

	paccmder, err := commands.NewPackageCommander(version.Current.Series)
	c.Assert(err, jc.ErrorIsNil)
	s.waitForFile(c, pacconfig.AptProxyConfigFile, paccmder.ProxyConfigContents(aptProxySettings)+"\n")
}

func (s *ProxyUpdaterSuite) TestEnvironmentVariables(c *gc.C) {
	setenv := func(proxy, value string) {
		os.Setenv(proxy, value)
		os.Setenv(strings.ToUpper(proxy), value)
	}
	setenv("http_proxy", "foo")
	setenv("https_proxy", "foo")
	setenv("ftp_proxy", "foo")
	setenv("no_proxy", "foo")

	proxySettings, _ := s.updateConfig(c)

	updater := proxyupdater.New(s.environmentAPI, true)
	defer worker.Stop(updater)
	s.waitForPostSetup(c)
	s.waitProxySettings(c, proxySettings)

	assertEnv := func(proxy, value string) {
		c.Assert(os.Getenv(proxy), gc.Equals, value)
		c.Assert(os.Getenv(strings.ToUpper(proxy)), gc.Equals, value)
	}
	assertEnv("http_proxy", proxySettings.Http)
	assertEnv("https_proxy", proxySettings.Https)
	assertEnv("ftp_proxy", proxySettings.Ftp)
	assertEnv("no_proxy", proxySettings.NoProxy)
}

func (s *ProxyUpdaterSuite) TestDontWriteSystemFiles(c *gc.C) {
	proxySettings, _ := s.updateConfig(c)

	updater := proxyupdater.New(s.environmentAPI, false)
	defer worker.Stop(updater)
	s.waitForPostSetup(c)

	s.waitProxySettings(c, proxySettings)
	c.Assert(pacconfig.AptProxyConfigFile, jc.DoesNotExist)
	c.Assert(s.proxyFile, jc.DoesNotExist)
}
