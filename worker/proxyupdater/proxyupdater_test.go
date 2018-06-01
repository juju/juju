// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater_test

import (
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/os/series"
	"github.com/juju/packaging/commands"
	pacconfig "github.com/juju/packaging/config"
	"github.com/juju/proxy"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	worker "gopkg.in/juju/worker.v1"

	proxyupdaterapi "github.com/juju/juju/api/proxyupdater"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker/proxyupdater"
	"github.com/juju/juju/worker/workertest"
)

type ProxyUpdaterSuite struct {
	coretesting.BaseSuite

	api              *fakeAPI
	proxyEnvFile     string
	proxySystemdFile string
	detectedSettings proxy.Settings
	inProcSettings   chan proxy.Settings
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
	proxies proxyupdaterapi.ProxyConfiguration
	Err     error
	Watcher *notAWatcher
}

func NewFakeAPI() *fakeAPI {
	f := &fakeAPI{}
	return f
}

func (api fakeAPI) ProxyConfig() (proxyupdaterapi.ProxyConfiguration, error) {
	return api.proxies, api.Err
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

	// Make buffer large for tests that never look at the settings.
	s.inProcSettings = make(chan proxy.Settings, 1000)

	directory := c.MkDir()
	s.proxySystemdFile = filepath.Join(directory, "systemd.file")
	s.proxyEnvFile = filepath.Join(directory, "env.file")

	s.config = proxyupdater.Config{
		SystemdFiles: []string{s.proxySystemdFile},
		EnvFiles:     []string{s.proxyEnvFile},
		API:          s.api,
		InProcessUpdate: func(newSettings proxy.Settings) error {
			select {
			case s.inProcSettings <- newSettings:
			case <-time.After(coretesting.LongWait):
				panic("couldn't send settings on inProcSettings channel")
			}
			return nil
		},
	}
	s.PatchValue(&pacconfig.AptProxyConfigFile, path.Join(directory, "juju-apt-proxy"))
}

func (s *ProxyUpdaterSuite) TearDownTest(c *gc.C) {
	s.BaseSuite.TearDownTest(c)
	if s.api.Watcher != nil {
		s.api.Watcher.Close()
	}
}

func (s *ProxyUpdaterSuite) waitProxySettings(c *gc.C, expected proxy.Settings) {
	maxWait := time.After(coretesting.LongWait)
	var (
		inProcSettings, envSettings proxy.Settings
		gotInProc, gotEnv           bool
	)
	for {
		select {
		case <-maxWait:
			c.Fatalf("timeout while waiting for proxy settings to change")
			return
		case inProcSettings = <-s.inProcSettings:
			if c.Check(inProcSettings, gc.Equals, expected) {
				gotInProc = true
			}
		case <-time.After(coretesting.ShortWait):
			envSettings = proxy.DetectProxies()
			if envSettings == expected {
				gotEnv = true
			} else {
				if envSettings != s.detectedSettings {
					c.Logf("proxy settings are \n%#v, should be \n%#v, still waiting", envSettings, expected)
				}
				s.detectedSettings = envSettings
			}
		}
		if gotEnv && gotInProc {
			break
		}
	}
}

func (s *ProxyUpdaterSuite) waitForFile(c *gc.C, filename, expected string) {
	//TODO(bogdanteleaga): Find a way to test this on windows
	if runtime.GOOS == "windows" {
		c.Skip("Proxy settings are written to the registry on windows")
	}
	maxWait := time.After(coretesting.LongWait)
	for {
		select {
		case <-maxWait:
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

func (s *ProxyUpdaterSuite) assertNoFile(c *gc.C, filename string) {
	//TODO(bogdanteleaga): Find a way to test this on windows
	if runtime.GOOS == "windows" {
		c.Skip("Proxy settings are written to the registry on windows")
	}
	maxWait := time.After(coretesting.ShortWait)
	for {
		select {
		case <-maxWait:
			return
		case <-time.After(10 * time.Millisecond):
			_, err := os.Stat(filename)
			if err == nil {
				c.Fatalf("file %s exists", filename)
			}
		}
	}
}

func (s *ProxyUpdaterSuite) TestRunStop(c *gc.C) {
	updater, err := proxyupdater.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CleanKill(c, updater)
}

func (s *ProxyUpdaterSuite) useLegacyConfig(c *gc.C) (proxy.Settings, proxy.Settings) {
	s.api.proxies = proxyupdaterapi.ProxyConfiguration{
		LegacyProxy: proxy.Settings{
			Http:    "http legacy proxy",
			Https:   "https legacy proxy",
			Ftp:     "ftp legacy proxy",
			NoProxy: "localhost,no legacy proxy",
		},
		APTProxy: proxy.Settings{
			Http:  "http://apt.http.proxy",
			Https: "https://apt.https.proxy",
			Ftp:   "ftp://apt.ftp.proxy",
		},
	}

	return s.api.proxies.LegacyProxy, s.api.proxies.APTProxy
}

func (s *ProxyUpdaterSuite) useJujuConfig(c *gc.C) (proxy.Settings, proxy.Settings) {
	s.api.proxies = proxyupdaterapi.ProxyConfiguration{
		JujuProxy: proxy.Settings{
			Http:    "http juju proxy",
			Https:   "https juju proxy",
			Ftp:     "ftp juju proxy",
			NoProxy: "localhost,no juju proxy",
		},
		APTProxy: proxy.Settings{
			Http:  "http://apt.http.proxy",
			Https: "https://apt.https.proxy",
			Ftp:   "ftp://apt.ftp.proxy",
		},
	}

	return s.api.proxies.JujuProxy, s.api.proxies.APTProxy
}

func (s *ProxyUpdaterSuite) TestInitialStateLegacyProxy(c *gc.C) {
	proxySettings, aptProxySettings := s.useLegacyConfig(c)

	updater, err := proxyupdater.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer worker.Stop(updater)

	s.waitProxySettings(c, proxySettings)
	s.waitForFile(c, s.proxyEnvFile, proxySettings.AsScriptEnvironment())
	s.waitForFile(c, s.proxySystemdFile, proxySettings.AsSystemdDefaultEnv())

	paccmder, err := commands.NewPackageCommander(series.MustHostSeries())
	c.Assert(err, jc.ErrorIsNil)
	s.waitForFile(c, pacconfig.AptProxyConfigFile, paccmder.ProxyConfigContents(aptProxySettings)+"\n")
}

func (s *ProxyUpdaterSuite) TestInitialStateJujuProxy(c *gc.C) {
	proxySettings, aptProxySettings := s.useJujuConfig(c)

	updater, err := proxyupdater.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer worker.Stop(updater)

	s.waitProxySettings(c, proxySettings)
	var empty proxy.Settings
	// The environment files are written, but with empty content.
	// This keeps the symlinks working.
	s.waitForFile(c, s.proxyEnvFile, empty.AsScriptEnvironment())
	s.waitForFile(c, s.proxySystemdFile, empty.AsSystemdDefaultEnv())

	paccmder, err := commands.NewPackageCommander(series.MustHostSeries())
	c.Assert(err, jc.ErrorIsNil)
	s.waitForFile(c, pacconfig.AptProxyConfigFile, paccmder.ProxyConfigContents(aptProxySettings)+"\n")
}

func (s *ProxyUpdaterSuite) TestEnvironmentVariablesLegacyProxy(c *gc.C) {
	setenv := func(proxy, value string) {
		os.Setenv(proxy, value)
		os.Setenv(strings.ToUpper(proxy), value)
	}
	setenv("http_proxy", "foo")
	setenv("https_proxy", "foo")
	setenv("ftp_proxy", "foo")
	setenv("no_proxy", "foo")

	proxySettings, _ := s.useLegacyConfig(c)
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

func (s *ProxyUpdaterSuite) TestEnvironmentVariablesJujuProxy(c *gc.C) {
	setenv := func(proxy, value string) {
		os.Setenv(proxy, value)
		os.Setenv(strings.ToUpper(proxy), value)
	}
	setenv("http_proxy", "foo")
	setenv("https_proxy", "foo")
	setenv("ftp_proxy", "foo")
	setenv("no_proxy", "foo")

	proxySettings, _ := s.useJujuConfig(c)
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

func (s *ProxyUpdaterSuite) TestExternalFuncCalled(c *gc.C) {

	// Called for both legacy and juju proxy values
	externalProxySet := func() proxy.Settings {
		updated := make(chan proxy.Settings)
		done := make(chan struct{})
		s.config.ExternalUpdate = func(values proxy.Settings) error {
			select {
			case updated <- values:
			case <-done:
			}
			return nil
		}
		updater, err := proxyupdater.NewWorker(s.config)
		c.Assert(err, jc.ErrorIsNil)
		defer worker.Stop(updater)
		// We need to close done before stopping the worker, so the
		// defer comes after the worker stop.
		defer close(done)

		select {
		case <-time.After(time.Second):
			c.Fatal("function not called")
		case externalSettings := <-updated:
			return externalSettings
		}
		return proxy.Settings{}
	}

	proxySettings, _ := s.useLegacyConfig(c)
	externalSettings := externalProxySet()
	c.Assert(externalSettings, jc.DeepEquals, proxySettings)

	proxySettings, _ = s.useJujuConfig(c)
	externalSettings = externalProxySet()
	c.Assert(externalSettings, jc.DeepEquals, proxySettings)
}

func (s *ProxyUpdaterSuite) TestErrorSettingInProcessLogs(c *gc.C) {
	proxySettings, _ := s.useJujuConfig(c)

	s.config.InProcessUpdate = func(newSettings proxy.Settings) error {
		select {
		case s.inProcSettings <- newSettings:
		case <-time.After(coretesting.LongWait):
			panic("couldn't send settings on inProcSettings channel")
		}
		return errors.New("gone daddy gone")
	}

	var logWriter loggo.TestWriter
	c.Assert(loggo.RegisterWriter("proxyupdater-tests", &logWriter), jc.ErrorIsNil)
	defer func() {
		loggo.RemoveWriter("proxyupdater-tests")
		logWriter.Clear()
	}()

	updater, err := proxyupdater.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	s.waitProxySettings(c, proxySettings)
	workertest.CleanKill(c, updater)

	var foundMessage bool
	expectedMessage := "error updating in-process proxy settings: gone daddy gone"
	for _, entry := range logWriter.Log() {
		if entry.Level == loggo.ERROR && strings.Contains(entry.Message, expectedMessage) {
			foundMessage = true
			break
		}
	}
	c.Assert(foundMessage, jc.IsTrue)
}
