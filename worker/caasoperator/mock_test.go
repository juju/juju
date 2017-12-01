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
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/watcher/watchertest"
	"github.com/juju/juju/worker/caasoperator"
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
