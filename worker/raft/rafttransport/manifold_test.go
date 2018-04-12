// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rafttransport_test

import (
	"context"
	"crypto/tls"
	"net"

	"github.com/hashicorp/raft"
	"github.com/juju/errors"
	"github.com/juju/pubsub"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/pubsub/centralhub"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	"github.com/juju/juju/worker/raft/rafttransport"
)

type ManifoldSuite struct {
	testing.IsolationSuite

	manifold dependency.Manifold
	context  dependency.Context
	agent    *mockAgent
	auth     *mockAuthenticator
	hub      *pubsub.StructuredHub
	mux      *apiserverhttp.Mux
	worker   worker.Worker
	stub     testing.Stub
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	tag := names.NewMachineTag("123")
	s.agent = &mockAgent{
		conf: mockAgentConfig{
			tag: tag,
			apiInfo: &api.Info{
				Addrs:  []string{"testing.invalid:1234"},
				CACert: coretesting.CACert,
			},
		},
	}
	s.auth = &mockAuthenticator{}
	s.hub = centralhub.New(tag)
	s.mux = &apiserverhttp.Mux{}
	s.stub.ResetCalls()
	s.worker = &mockTransportWorker{
		Transport: &raft.InmemTransport{},
	}

	s.context = s.newContext(nil)
	s.manifold = rafttransport.Manifold(rafttransport.ManifoldConfig{
		AgentName:         "agent",
		AuthenticatorName: "auth",
		HubName:           "hub",
		MuxName:           "mux",
		DialConn:          s.dialConn,
		NewWorker:         s.newWorker,
		Path:              "raft/path",
	})
}

func (s *ManifoldSuite) newContext(overlay map[string]interface{}) dependency.Context {
	resources := map[string]interface{}{
		"agent": s.agent,
		"auth":  s.auth,
		"hub":   s.hub,
		"mux":   s.mux,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubContext(nil, resources)
}

func (s *ManifoldSuite) newWorker(config rafttransport.Config) (worker.Worker, error) {
	s.stub.MethodCall(s, "NewWorker", config)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return s.worker, nil
}

func (s *ManifoldSuite) dialConn(ctx context.Context, addr string, tlsConfig *tls.Config) (net.Conn, error) {
	s.stub.MethodCall(s, "DialConn")
	return nil, s.stub.NextErr()
}

var expectedInputs = []string{"agent", "auth", "hub", "mux"}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Assert(s.manifold.Inputs, jc.SameContents, expectedInputs)
}

func (s *ManifoldSuite) TestMissingInputs(c *gc.C) {
	for _, input := range expectedInputs {
		context := s.newContext(map[string]interface{}{
			input: dependency.ErrMissing,
		})
		_, err := s.manifold.Start(context)
		c.Assert(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	}
}

func (s *ManifoldSuite) TestStart(c *gc.C) {
	s.startWorkerClean(c)

	s.stub.CheckCallNames(c, "NewWorker")
	args := s.stub.Calls()[0].Args
	c.Assert(args, gc.HasLen, 1)
	c.Assert(args[0], gc.FitsTypeOf, rafttransport.Config{})
	config := args[0].(rafttransport.Config)

	c.Assert(config.DialConn, gc.NotNil)
	config.DialConn(context.Background(), "foo", &tls.Config{})
	s.stub.CheckCallNames(c, "NewWorker", "DialConn")
	config.DialConn = nil

	c.Assert(config.TLSConfig, gc.NotNil)
	config.TLSConfig = nil

	c.Assert(config, jc.DeepEquals, rafttransport.Config{
		APIInfo: &api.Info{
			Addrs:  []string{"testing.invalid:1234"},
			CACert: coretesting.CACert,
		},
		Hub:           s.hub,
		Mux:           s.mux,
		Authenticator: s.auth,
		Path:          "raft/path",
		Tag:           s.agent.conf.tag,
	})
}

func (s *ManifoldSuite) TestOutput(c *gc.C) {
	w := s.startWorkerClean(c)

	var t raft.Transport
	err := s.manifold.Output(w, &t)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(t, gc.Equals, s.worker)
}

func (s *ManifoldSuite) startWorkerClean(c *gc.C) worker.Worker {
	w, err := s.manifold.Start(s.context)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.Equals, s.worker)
	return w
}
