// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitinit_test

import (
	"github.com/juju/testing"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/caasunitinit"
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
	caasunitinit.Client
	ContainerStartWatcher *watchertest.MockStringsWatcher
}

func (c *fakeClient) WatchContainerStart(application string, container string) (watcher.StringsWatcher, error) {
	c.MethodCall(c, "WatchContainerStart", application, container)
	if err := c.NextErr(); err != nil {
		return nil, err
	}
	return c.ContainerStartWatcher, nil
}
