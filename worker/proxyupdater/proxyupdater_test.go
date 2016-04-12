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
	proxyutils "github.com/juju/utils/proxy"
	"github.com/juju/utils/series"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/proxyupdater"
	"github.com/juju/juju/worker/workertest"
)

type ProxyUpdaterSuite struct {
	coretesting.BaseSuite

	api              *fakeAPI
	proxyFile        string
	detectedSettings proxy.Settings
	config           proxyupdater.Config
}

var _ = gc.Suite(&ProxyUpdaterSuite{})

func newNotAWatcher() notAWatcher {
	return notAWatcher{workertest.NewFakeWatcher(2, 2)}
}

type notAWatcher struct {
	workertest.NotAWatcher
}

func (w notAWatcher) Changes() watcher.NotifyChannel {
	return w.NotAWatcher.Changes()
}

type fakeAPI struct {
	Proxy    proxyutils.Settings
	APTProxy proxyutils.Settings
	Err      error
	Watcher  *notAWatcher
}

func NewFakeAPI() *fakeAPI {
	f := &fakeAPI{}
	return f
}

func (api fakeAPI) ProxyConfig() (proxyutils.Settings, proxyutils.Settings, error) {
	return api.Proxy, api.APTProxy, api.Err

}

func (api fakeAPI) WatchForProxyConfigAndAPIHostPortChanges() (watcher.NotifyWatcher, error) {
	if api.Watcher == nil {
		w := newNotAWatcher()
		api.Watcher = &w
	}
	return api.Watcher, nil
}

func (s *ProxyUpdaterSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.api = NewFakeAPI()

	s.config.Directory = c.MkDir()
	s.config.Filename = "juju-proxy-settings"
	s.config.API = s.api
	s.PatchValue(&pacconfig.AptProxyConfigFile, path.Join(s.config.Directory, "juju-apt-proxy"))
	s.proxyFile = path.Join(s.config.Directory, s.config.Filename)
}

func (s *ProxyUpdaterSuite) TearDownTest(c *gc.C) {
	s.BaseSuite.TearDownTest(c)
	if s.api.Watcher != nil {
		s.api.Watcher.Close()
	}
}

func (s *ProxyUpdaterSuite) waitProxySettings(c *gc.C, expected proxy.Settings) {
	for {
		select {
		case <-time.After(time.Second):
			c.Fatalf("timeout while waiting for proxy settings to change")
			return
		case <-time.After(10 * time.Millisecond):
			obtained := proxy.DetectProxies()
			if obtained != expected {
				if obtained != s.detectedSettings {
					c.Logf("proxy settings are \n%#v, should be \n%#v, still waiting", obtained, expected)
				}
				s.detectedSettings = obtained
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
		case <-time.After(time.Second):
			c.Fatalf("timeout while waiting for proxy settings to change")
			return
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
	updater, err := proxyupdater.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CleanKill(c, updater)
}

func (s *ProxyUpdaterSuite) updateConfig(c *gc.C) (proxy.Settings, proxy.Settings) {
	s.api.Proxy = proxy.Settings{
		Http:    "http proxy",
		Https:   "https proxy",
		Ftp:     "ftp proxy",
		NoProxy: "localhost,no proxy",
	}

	s.api.APTProxy = proxy.Settings{
		Http:  "http://apt.http.proxy",
		Https: "https://apt.https.proxy",
		Ftp:   "ftp://apt.ftp.proxy",
	}

	return s.api.Proxy, s.api.APTProxy
}

func (s *ProxyUpdaterSuite) TestInitialState(c *gc.C) {
	proxySettings, aptProxySettings := s.updateConfig(c)

	updater, err := proxyupdater.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer worker.Stop(updater)

	s.waitProxySettings(c, proxySettings)
	s.waitForFile(c, s.proxyFile, proxySettings.AsScriptEnvironment()+"\n")

	paccmder, err := commands.NewPackageCommander(series.HostSeries())
	c.Assert(err, jc.ErrorIsNil)
	s.waitForFile(c, pacconfig.AptProxyConfigFile, paccmder.ProxyConfigContents(aptProxySettings)+"\n")
}

func (s *ProxyUpdaterSuite) TestWriteSystemFiles(c *gc.C) {
	proxySettings, aptProxySettings := s.updateConfig(c)

	updater, err := proxyupdater.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer worker.Stop(updater)

	s.waitProxySettings(c, proxySettings)
	s.waitForFile(c, s.proxyFile, proxySettings.AsScriptEnvironment()+"\n")

	paccmder, err := commands.NewPackageCommander(series.HostSeries())
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
	updater, err := proxyupdater.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer worker.Stop(updater)
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
