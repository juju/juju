// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator_test

import (
	"github.com/juju/testing"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/downloader"
	"github.com/juju/juju/status"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/watcher/watchertest"
	"github.com/juju/juju/worker/caasoperator"
	"github.com/juju/juju/worker/caasoperator/hook"
	"github.com/juju/juju/worker/caasoperator/runner"
	"github.com/juju/juju/worker/caasoperator/runner/context"
)

var (
	gitlabCharmURL = charm.MustParseURL("cs:gitlab-1")
	gitlabSettings = charm.Settings{"k": 123}

	fakeCharmContent = []byte("abc")
	fakeCharmSHA256  = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
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
	settingsWatcher *watchertest.MockNotifyWatcher
}

func (c *fakeClient) SetStatus(application string, status status.Status, message string, data map[string]interface{}) error {
	c.MethodCall(c, "SetStatus", application, status, message, data)
	return c.NextErr()
}

func (c *fakeClient) Charm(application string) (*charm.URL, string, error) {
	c.MethodCall(c, "Charm", application)
	if err := c.NextErr(); err != nil {
		return nil, "", err
	}
	return gitlabCharmURL, fakeCharmSHA256, nil
}

func (c *fakeClient) SetContainerSpec(entityName, spec string) error {
	c.MethodCall(c, "SetContainerSpec", entityName, spec)
	return c.NextErr()
}

func (c *fakeClient) ApplicationConfig(application string) (charm.Settings, error) {
	c.MethodCall(c, "ApplicationConfig", application)
	if err := c.NextErr(); err != nil {
		return nil, err
	}
	return gitlabSettings, nil
}

func (c *fakeClient) WatchApplicationConfig(application string) (watcher.NotifyWatcher, error) {
	c.MethodCall(c, "WatchApplicationConfig", application)
	if err := c.NextErr(); err != nil {
		return nil, err
	}
	return c.settingsWatcher, nil
}

func (c *fakeClient) ModelName() (string, error) {
	return "gitlab-model", nil
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

func newRunnerFactoryFunc(observer *hookObserver) runner.NewRunnerFactoryFunc {
	return func(context.Paths, context.ContextFactory) (runner.Factory, error) {
		return &mockRunnerFactory{observer: observer}, nil
	}
}

type mockRunnerFactory struct {
	runner.Factory
	observer *hookObserver
}

func (m *mockRunnerFactory) NewHookRunner(hookInfo hook.Info) (runner.Runner, error) {
	return &mockHookRunner{observer: m.observer}, nil
}

type mockHookRunner struct {
	runner.Runner
	observer *hookObserver
}

func (m *mockHookRunner) Context() runner.Context {
	return &context.HookContext{}
}

func (m *mockHookRunner) RunHook(name string) error {
	if m.observer == nil {
		return nil
	}
	m.observer.hooksCompleted = append(m.observer.hooksCompleted, name)
	return nil
}
