// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator_test

import (
	"github.com/juju/errors"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/worker/fortress"
	"github.com/juju/testing"
	"github.com/juju/utils/proxy"
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

type fakeLeadershipTracker struct {
	unitTag names.UnitTag
}

//func (t *fakeLeadershipTracker) ApplicationName() string {
//
//}
//
//func (t *fakeLeadershipTracker) ClaimDuration() time.Duration {
//
//}
//
//func (t *fakeLeadershipTracker) ClaimLeader() Ticket {
//
//}
//
//func (t *fakeLeadershipTracker) WaitLeader() Ticket {
//
//}
//
//// WaitMinion will return a Ticket which, when Wait()ed for, will block
//// until the tracker's future leadership can no longer be guaranteed.
//WaitMinion() Ticket

type fakeAPICaller struct {
	base.APICaller
}

type fakeClient struct {
	testing.Stub
	caasoperator.Client
	unitsWatcher *watchertest.MockStringsWatcher
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

func (c *fakeClient) WatchUnits(application string) (watcher.StringsWatcher, error) {
	c.MethodCall(c, "WatchUnits", application)
	if err := c.NextErr(); err != nil {
		return nil, err
	}
	return c.unitsWatcher, nil
}

func (c *fakeClient) Life(entity string) (life.Value, error) {
	c.MethodCall(c, "Life", entity)
	if err := c.NextErr(); err != nil {
		return life.Dead, err
	}
	return life.Alive, nil
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
	m.observer.recordHookCompleted(name)
	return nil
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
