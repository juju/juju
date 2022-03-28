// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raft_test

import (
	"github.com/hashicorp/raft"
	"github.com/juju/worker/v3"

	"github.com/juju/juju/agent"
	"github.com/juju/names/v4"
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
	dataDir                  string
	tag                      names.Tag
	nonSyncedWritesToRaftLog bool
	batchRaftFSM             bool
}

func (c *mockAgentConfig) Tag() names.Tag {
	return c.tag
}

func (c *mockAgentConfig) DataDir() string {
	return c.dataDir
}

func (c *mockAgentConfig) NonSyncedWritesToRaftLog() bool {
	return c.nonSyncedWritesToRaftLog
}

func (c *mockAgentConfig) BatchRaftFSM() bool {
	return c.batchRaftFSM
}

type mockRaftWorker struct {
	worker.Worker
	testing.Stub
	r  *raft.Raft
	ls raft.LogStore
}

func (r *mockRaftWorker) Raft() (*raft.Raft, error) {
	r.MethodCall(r, "Raft")
	return r.r, r.NextErr()
}

func (r *mockRaftWorker) LogStore() (raft.LogStore, error) {
	r.MethodCall(r, "LogStore")
	return r.ls, r.NextErr()
}

func (r *mockRaftWorker) Kill() {
	r.MethodCall(r, "Kill")
}

func (r *mockRaftWorker) Wait() error {
	r.MethodCall(r, "Wait")
	return r.NextErr()
}

type mockLogStore struct {
	raft.LogStore
}
