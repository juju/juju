// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rafttransport_test

import (
	"github.com/hashicorp/raft"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v3"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
)

type mockAgent struct {
	agent.Agent
	conf mockAgentConfig
}

func (ma *mockAgent) CurrentConfig() agent.Config {
	return &ma.conf
}

type mockAgentConfig struct {
	agent.Config
	tag     names.Tag
	apiInfo *api.Info
}

func (c *mockAgentConfig) Tag() names.Tag {
	return c.tag
}

func (c *mockAgentConfig) APIInfo() (*api.Info, bool) {
	return c.apiInfo, c.apiInfo != nil
}

type mockTransportWorker struct {
	worker.Worker
	raft.Transport
}
