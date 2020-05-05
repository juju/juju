// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater_test

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
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
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	proxyupdaterapi "github.com/juju/juju/api/proxyupdater"
	"github.com/juju/juju/core/watcher"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/proxyupdater"
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

func (api *fakeAPI) WatchForProxyConfigAndAPIHostPortChanges() (watcher.NotifyWatcher, error) {
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
	logger := loggo.GetLogger("test.proxyupdater")
	logger.SetLogLevel(loggo.TRACE)
	s.config = proxyupdater.Config{
		SupportLegacyValues: true,
		SystemdFiles:        []string{s.proxySystemdFile},
		EnvFiles:            []string{s.proxyEnvFile},
		API:                 s.api,
		InProcessUpdate: func(newSettings proxy.Settings) error {
			select {
			case s.inProcSettings <- newSettings:
			case <-time.After(coretesting.LongWait):
				panic("couldn't send settings on inProcSettings channel")
			}
			return nil
		},
		Logger: logger,
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

func nextCall(c *gc.C, calls <-chan []string) []string {
	select {
	case call := <-calls:
		return call
	case <-time.After(coretesting.LongWait):
		c.Fatalf("run func not called")
	}
	panic("unreachable")
}

func (s *ProxyUpdaterSuite) TestSnapProxySetNoneSet(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("snap settings not handled on windows")
	}

	logger := s.config.Logger
	calls := make(chan []string)
	s.config.RunFunc = func(in string, cmd string, args ...string) (string, error) {
		logger.Debugf("RunFunc(%q, %q, %#v)", in, cmd, args)
		calls <- append([]string{in, cmd}, args...)
		return "", nil
	}

	s.api.proxies = proxyupdaterapi.ProxyConfiguration{}

	updater, err := proxyupdater.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, updater)

	// The worker doesn't precheck any of the snap proxy values, as it is expected
	// that the set call is cheap. Every time the worker starts, we call set for the current
	// values.
	c.Assert(nextCall(c, calls), jc.DeepEquals, []string{"", "snap", "set", "core",
		"proxy.http=",
		"proxy.https=",
		"proxy.store=",
	})
}

func (s *ProxyUpdaterSuite) TestSnapProxySet(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("snap settings not handled on windows")
	}

	logger := s.config.Logger
	calls := make(chan []string)
	s.config.RunFunc = func(in string, cmd string, args ...string) (string, error) {
		logger.Debugf("RunFunc(%q, %q, %#v)", in, cmd, args)
		calls <- append([]string{in, cmd}, args...)
		return "", nil
	}

	s.api.proxies = proxyupdaterapi.ProxyConfiguration{
		SnapProxy: proxy.Settings{
			Http:  "http://snap-proxy",
			Https: "https://snap-proxy",
		},
	}

	updater, err := proxyupdater.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, updater)

	// The snap store is set to the empty string because as the agent is starting
	// and it doesn't check to see what the store was set to, so to be sure, it just
	// calls the set value.
	c.Assert(nextCall(c, calls), jc.DeepEquals, []string{"", "snap", "set", "core",
		"proxy.http=http://snap-proxy",
		"proxy.https=https://snap-proxy",
		"proxy.store=",
	})
}

func (s *ProxyUpdaterSuite) TestSnapStoreProxy(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("snap settings not handled on windows")
	}

	logger := s.config.Logger
	calls := make(chan []string)
	s.config.RunFunc = func(in string, cmd string, args ...string) (string, error) {
		logger.Debugf("RunFunc(%q, %q, %#v)", in, cmd, args)
		calls <- append([]string{in, cmd}, args...)
		return "", nil
	}

	s.api.proxies = proxyupdaterapi.ProxyConfiguration{
		SnapStoreProxyId:         "42",
		SnapStoreProxyAssertions: "please trust us",
	}

	updater, err := proxyupdater.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, updater)

	c.Assert(nextCall(c, calls), jc.DeepEquals, []string{"please trust us", "snap", "ack", "/dev/stdin"})

	// The http and https proxy values are set to be empty as it is the first pass through.
	c.Assert(nextCall(c, calls), jc.DeepEquals, []string{"", "snap", "set", "core",
		"proxy.http=",
		"proxy.https=",
		"proxy.store=42",
	})
}

func (s *ProxyUpdaterSuite) TestSnapStoreProxyURL(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("snap settings not handled on windows")
	}

	logger := s.config.Logger
	calls := make(chan []string)
	s.config.RunFunc = func(in string, cmd string, args ...string) (string, error) {
		logger.Debugf("RunFunc(%q, %q, %#v)", in, cmd, args)
		calls <- append([]string{in, cmd}, args...)
		return "", nil
	}

	var (
		srv *httptest.Server

		proxyRes = `
type: store
authority-id: canonical
store: WhatDoesTheBigRedButtonDo
operator-id: 0123456789067OdMqoW9YLp3e0EgakQf
timestamp: 2019-08-27T12:20:45.166790Z
url: $url
sign-key-sha3-384: BWDEoaqyr25nF5SNCvEv2v7QnM9QsfCc0PBMYD_i2NGSQ32EF2d4D0hqUel3m8ul

DATA...
DATA...
`
	)

	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(proxyRes))
	}))
	proxyRes = strings.Replace(proxyRes, "$url", srv.URL, -1)
	defer srv.Close()

	s.api.proxies = proxyupdaterapi.ProxyConfiguration{
		SnapStoreProxyURL: srv.URL,
	}

	updater, err := proxyupdater.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, updater)

	c.Assert(nextCall(c, calls), jc.DeepEquals, []string{proxyRes, "snap", "ack", "/dev/stdin"})

	// The http and https proxy values are set to be empty as it is the first pass through.
	c.Assert(nextCall(c, calls), jc.DeepEquals, []string{"", "snap", "set", "core",
		"proxy.http=",
		"proxy.https=",
		"proxy.store=WhatDoesTheBigRedButtonDo",
	})
}

func (s *ProxyUpdaterSuite) TestSnapStoreProxyURLOverridesManualAssertion(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("snap settings not handled on windows")
	}

	logger := s.config.Logger
	calls := make(chan []string)
	s.config.RunFunc = func(in string, cmd string, args ...string) (string, error) {
		logger.Debugf("RunFunc(%q, %q, %#v)", in, cmd, args)
		calls <- append([]string{in, cmd}, args...)
		return "", nil
	}

	var (
		srv *httptest.Server

		proxyRes = `
type: store
authority-id: canonical
store: WhatDoesTheBigRedButtonDo
operator-id: 0123456789067OdMqoW9YLp3e0EgakQf
timestamp: 2019-08-27T12:20:45.166790Z
url: $url
sign-key-sha3-384: BWDEoaqyr25nF5SNCvEv2v7QnM9QsfCc0PBMYD_i2NGSQ32EF2d4D0hqUel3m8ul

DATA...
DATA...
`
	)

	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(proxyRes))
	}))
	proxyRes = strings.Replace(proxyRes, "$url", srv.URL, -1)
	defer srv.Close()

	s.api.proxies = proxyupdaterapi.ProxyConfiguration{
		SnapStoreProxyId:         "42",
		SnapStoreProxyAssertions: "please trust us",
		SnapStoreProxyURL:        srv.URL,
	}

	updater, err := proxyupdater.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, updater)

	c.Assert(nextCall(c, calls), jc.DeepEquals, []string{proxyRes, "snap", "ack", "/dev/stdin"})

	// The http and https proxy values are set to be empty as it is the first pass through.
	c.Assert(nextCall(c, calls), jc.DeepEquals, []string{"", "snap", "set", "core",
		"proxy.http=",
		"proxy.https=",
		"proxy.store=WhatDoesTheBigRedButtonDo",
	})
}
