// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raft_test

import (
	"github.com/hashicorp/raft"
	"gopkg.in/juju/names.v2"
	worker "gopkg.in/juju/worker.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/testing"
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
	dataDir string
	tag     names.Tag
}

func (c *mockAgentConfig) Tag() names.Tag {
	return c.tag
}

func (c *mockAgentConfig) DataDir() string {
	return c.dataDir
}

type mockRaftWorker struct {
	worker.Worker
	testing.Stub
	r *raft.Raft
}

func (r *mockRaftWorker) Raft() (*raft.Raft, error) {
	r.MethodCall(r, "Raft")
	return r.r, r.NextErr()
}
