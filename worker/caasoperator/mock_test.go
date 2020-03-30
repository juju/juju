// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator_test

import (
	"github.com/juju/errors"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/worker/fortress"
	"github.com/juju/proxy"
	"github.com/juju/testing"
	"github.com/juju/version"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	caasoperatorapi "github.com/juju/juju/api/caasoperator"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/downloader"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/caasoperator"
)

var (
	gitlabCharmURL = charm.MustParseURL("cs:gitlab-1")
	gitlabSettings = charm.Settings{"k": 123}

	fakeCharmContent    = []byte("abc")
	fakeCharmSHA256     = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	fakeModifiedVersion = 666
)

type fakeAgent struct {
	agent.Agent
	config fakeAgentConfig
}

func (a *fakeAgent) CurrentConfig() agent.Config {
	return &a.config
}

type fakeAgentConfig struct {
	agent.Config
	dataDir string
	tag     names.Tag
}

func (c *fakeAgentConfig) Tag() names.Tag {
	return c.tag
}

func (c *fakeAgentConfig) Model() names.ModelTag {
	return coretesting.ModelTag
}

func (c *fakeAgentConfig) DataDir() string {
	return c.dataDir
}

type fakeAPICaller struct {
	base.APICaller
}

type fakeClient struct {
	testing.Stub
	caasoperator.Client
	unitsWatcher       *watchertest.MockStringsWatcher
	containerWatcher   *watchertest.MockStringsWatcher
	watcher            *watchertest.MockNotifyWatcher
	applicationWatched chan struct{}
	unitRemoved        chan struct{}
	life               life.Value
	mode               caas.DeploymentMode
}

func (c *fakeClient) SetStatus(application string, status status.Status, message string, data map[string]interface{}) error {
	c.MethodCall(c, "SetStatus", application, status, message, data)
	return c.NextErr()
}

func (c *fakeClient) Charm(application string) (*caasoperatorapi.CharmInfo, error) {
	c.MethodCall(c, "Charm", application)
	if err := c.NextErr(); err != nil {
		return nil, err
	}
	return &caasoperatorapi.CharmInfo{
		URL:                  gitlabCharmURL,
		ForceUpgrade:         true,
		SHA256:               fakeCharmSHA256,
		CharmModifiedVersion: fakeModifiedVersion,
		DeploymentMode:       c.mode,
	}, nil
}

func (c *fakeClient) CharmConfig(application string) (charm.Settings, error) {
	c.MethodCall(c, "CharmConfig", application)
	if err := c.NextErr(); err != nil {
		return nil, err
	}
	return gitlabSettings, nil
}

func (c *fakeClient) WatchCharmConfig(application string) (watcher.NotifyWatcher, error) {
	return nil, errors.NotSupportedf("watch charm config")
}

func (c *fakeClient) WatchContainerStart(application, container string) (watcher.StringsWatcher, error) {
	c.MethodCall(c, "WatchContainerStart", application, container)
	if err := c.NextErr(); err != nil {
		return nil, err
	}
	return c.containerWatcher, nil
}

func (c *fakeClient) WatchUnits(application string) (watcher.StringsWatcher, error) {
	c.MethodCall(c, "WatchUnits", application)
	if err := c.NextErr(); err != nil {
		return nil, err
	}
	return c.unitsWatcher, nil
}

func (c *fakeClient) Watch(application string) (watcher.NotifyWatcher, error) {
	c.MethodCall(c, "Watch", application)
	if err := c.NextErr(); err != nil {
		return nil, err
	}
	c.applicationWatched <- struct{}{}
	return c.watcher, nil
}

func (c *fakeClient) RemoveUnit(unit string) error {
	c.MethodCall(c, "RemoveUnit", unit)
	c.unitRemoved <- struct{}{}
	return c.NextErr()
}

func (c *fakeClient) SetVersion(appName string, v version.Binary) error {
	c.MethodCall(c, "SetVersion", appName, v)
	return c.NextErr()
}

func (c *fakeClient) Life(entity string) (life.Value, error) {
	c.MethodCall(c, "Life", entity)
	if err := c.NextErr(); err != nil {
		return life.Dead, err
	}
	return c.life, nil
}

func (c *fakeClient) APIAddresses() ([]string, error) {
	c.MethodCall(c, "APIAddresses", nil)
	if err := c.NextErr(); err != nil {
		return nil, err
	}
	return []string{"10.0.0.1:10000"}, nil
}

func (c *fakeClient) ProxySettings() (proxy.Settings, error) {
	c.MethodCall(c, "ProxySettings", nil)
	if err := c.NextErr(); err != nil {
		return proxy.Settings{}, err
	}
	return proxy.Settings{Http: "http.proxy"}, nil
}

func (c *fakeClient) Model() (*model.Model, error) {
	return &model.Model{
		Name: "gitlab-model",
	}, nil
}

type fakeDownloader struct {
	testing.Stub
	path string
}

func (d *fakeDownloader) Download(req downloader.Request) (string, error) {
	d.MethodCall(d, "Download", req)
	if err := d.NextErr(); err != nil {
		return "", err
	}
	return d.path, nil
}

type mockCharmDirGuard struct {
	fortress.Guard
	testing.Stub
}

func (l *mockCharmDirGuard) Unlock() error {
	l.MethodCall(l, "Unlock")
	return l.NextErr()
}

func (l *mockCharmDirGuard) Lockdown(abort fortress.Abort) error {
	l.MethodCall(l, "Lockdown", abort)
	return l.NextErr()
}

type mockHookLogger struct {
	stopped bool
}

func (m *mockHookLogger) Stop() {
	m.stopped = true
}
