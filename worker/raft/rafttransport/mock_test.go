// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rafttransport_test

import (
	"context"
	"net"

	"github.com/hashicorp/raft"
	"github.com/juju/worker/v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
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

type mockAPIConnection struct {
	api.Connection
	testing.Stub
	dialContext func(context.Context) (net.Conn, error)
}

func (c *mockAPIConnection) Close() error {
	c.MethodCall(c, "Close")
	return c.NextErr()
}

func (c *mockAPIConnection) DialConn(ctx context.Context) (net.Conn, error) {
	c.MethodCall(c, "DialConn", ctx)
	if err := c.NextErr(); err != nil {
		return nil, err
	}
	return c.dialContext(ctx)
}
