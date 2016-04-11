// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater_test

import (
	"errors"
	"io/ioutil"
	"os"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/packaging/commands"
	pacconfig "github.com/juju/utils/packaging/config"
	"github.com/juju/utils/proxy"
	proxyutils "github.com/juju/utils/proxy"
	"github.com/juju/utils/series"
	gc "gopkg.in/check.v1"

	"net/url"

	"github.com/juju/httprequest"
	"github.com/juju/juju/api/base"
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
}

var _ = gc.Suite(&ProxyUpdaterSuite{})

type wkr struct {
	Channel chan struct{}
}

func newWkr() wkr {
	return wkr{
		Channel: make(chan struct{}, 2),
	}
}

func (wkr) Kill() {
}

func (wkr) Wait() error {
	return nil
}

func (w wkr) Changes() watcher.NotifyChannel {
	return w.Channel
}

func (w *wkr) Ping() {
	w.Channel <- struct{}{}
}

type fakeAPI struct {
	Proxy    proxyutils.Settings
	APTProxy proxyutils.Settings
	Err      error
	Watcher  wkr
}

func NewFakeAPI() *fakeAPI {
	f := &fakeAPI{
		Watcher: newWkr(),
	}
	return f
}

func (api fakeAPI) ProxyConfig() (proxyutils.Settings, proxyutils.Settings, error) {
	return api.Proxy, api.APTProxy, api.Err

}

func (api fakeAPI) WatchForProxyConfigAndAPIHostPortChanges() (watcher.NotifyWatcher, error) {
	return api.Watcher, nil
}

func (fakeAPI) APICall(objType string, version int, id, request string, params, response interface{}) error {
	return nil
}

func (fakeAPI) BestFacadeVersion(facade string) int {
	return 32
}

func (fakeAPI) ModelTag() (names.ModelTag, error) {
	return coretesting.ModelTag, nil
}

func (fakeAPI) Close() error {
	return nil
}

func (fakeAPI) HTTPClient() (*httprequest.Client, error) {
	return nil, errors.New("no HTTP client available in this test")
}

func (fakeAPI) ConnectStream(path string, attrs url.Values) (base.Stream, error) {
	return nil, errors.New("stream connection unimplemented")
}

func (s *ProxyUpdaterSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.api = NewFakeAPI()

	proxyDir := c.MkDir()
	s.PatchValue(&proxyupdater.ProxyDirectory, proxyDir)
	s.PatchValue(&pacconfig.AptProxyConfigFile, path.Join(proxyDir, "juju-apt-proxy"))
	s.proxyFile = path.Join(proxyDir, proxyupdater.ProxyFile)
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
	updater, err := proxyupdater.NewWorker(s.api)
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

	s.api.Watcher.Ping()

	return s.api.Proxy, s.api.APTProxy
}

func (s *ProxyUpdaterSuite) TestInitialState(c *gc.C) {
	proxySettings, aptProxySettings := s.updateConfig(c)

	updater, err := proxyupdater.NewWorker(s.api)
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

	updater, err := proxyupdater.NewWorker(s.api)
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
	updater, err := proxyupdater.NewWorker(s.api)
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
