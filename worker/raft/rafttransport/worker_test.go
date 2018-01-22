// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rafttransport_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/hashicorp/raft"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/pubsub/apiserver"
	"github.com/juju/juju/pubsub/centralhub"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/raft/rafttransport"
	"github.com/juju/juju/worker/workertest"
)

var controllerTag = names.NewMachineTag("123")

type workerFixture struct {
	testing.IsolationSuite
	config   rafttransport.Config
	authInfo apiserverhttp.AuthInfo
}

func (s *workerFixture) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	tag := names.NewMachineTag("123")
	s.config = rafttransport.Config{
		APIInfo: &api.Info{
			Tag:      tag,
			Password: "valid-password",
		},
		APIOpen: api.Open,
		Hub:     centralhub.New(tag),
		Mux: apiserverhttp.NewMux(
			apiserverhttp.WithAuth(s.auth),
		),
		Path:    "/raft/path",
		Tag:     tag,
		Timeout: coretesting.LongWait,
	}

	logger := loggo.GetLogger("juju.worker.raft")
	oldLevel := logger.LogLevel()
	logger.SetLogLevel(loggo.TRACE)
	s.AddCleanup(func(c *gc.C) {
		logger.SetLogLevel(oldLevel)
	})
}

func (s *workerFixture) auth(req *http.Request) (apiserverhttp.AuthInfo, error) {
	user, pass, ok := req.BasicAuth()
	if !ok || pass != "valid-password" {
		return apiserverhttp.AuthInfo{}, errors.Unauthorizedf("request")
	}
	tag, err := names.ParseTag(user)
	if err != nil {
		return apiserverhttp.AuthInfo{}, errors.Trace(err)
	}
	return apiserverhttp.AuthInfo{
		Tag:        tag,
		Controller: tag == controllerTag,
	}, nil
}

type WorkerValidationSuite struct {
	workerFixture
}

var _ = gc.Suite(&WorkerValidationSuite{})

func (s *WorkerValidationSuite) TestValidateErrors(c *gc.C) {
	type test struct {
		f      func(*rafttransport.Config)
		expect string
	}
	tests := []test{{
		func(cfg *rafttransport.Config) { cfg.APIInfo = nil },
		"nil APIInfo not valid",
	}, {
		func(cfg *rafttransport.Config) { cfg.APIOpen = nil },
		"nil APIOpen not valid",
	}, {
		func(cfg *rafttransport.Config) { cfg.Hub = nil },
		"nil Hub not valid",
	}, {
		func(cfg *rafttransport.Config) { cfg.Mux = nil },
		"nil Mux not valid",
	}, {
		func(cfg *rafttransport.Config) { cfg.Path = "" },
		"empty Path not valid",
	}, {
		func(cfg *rafttransport.Config) { cfg.Tag = nil },
		"nil Tag not valid",
	}}
	for i, test := range tests {
		c.Logf("test #%d (%s)", i, test.expect)
		s.testValidateError(c, test.f, test.expect)
	}
}

func (s *WorkerValidationSuite) testValidateError(c *gc.C, f func(*rafttransport.Config), expect string) {
	config := s.config
	f(&config)
	w, err := rafttransport.NewWorker(config)
	if !c.Check(err, gc.NotNil) {
		workertest.DirtyKill(c, w)
		return
	}
	c.Check(w, gc.IsNil)
	c.Check(err, gc.ErrorMatches, expect)
}

type WorkerSuite struct {
	workerFixture
	stub    testing.Stub
	apiConn *mockAPIConnection
	server  *httptest.Server
	worker  *rafttransport.Worker
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) SetUpTest(c *gc.C) {
	s.workerFixture.SetUpTest(c)

	s.stub.ResetCalls()
	s.server = httptest.NewServer(s.config.Mux)
	s.AddCleanup(func(c *gc.C) {
		s.server.Close()
	})
	dialContext := func(ctx context.Context) (net.Conn, error) {
		var dialer net.Dialer
		addr := s.server.Listener.Addr()
		return dialer.DialContext(ctx, addr.Network(), addr.String())
	}
	s.apiConn = &mockAPIConnection{
		dialContext: dialContext,
	}
	s.config.APIOpen = s.apiOpen
	s.worker = s.newWorker(c, s.config)
	s.config.Hub.Publish(apiserver.DetailsTopic, apiserver.Details{
		Servers: map[string]apiserver.APIServer{
			"123": apiserver.APIServer{
				ID:        "123",
				Addresses: []string{"testing.invalid:1234"},
			},
		},
	})
}

// newWorker returns a new rafttransport.Worker. The caller is expected to
// publish apiserver.Details changes to the hub after the worker starts.
func (s *WorkerSuite) newWorker(c *gc.C, config rafttransport.Config) *rafttransport.Worker {
	worker, err := rafttransport.NewWorker(config)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) {
		workertest.DirtyKill(c, worker)
	})
	return worker
}

func (s *WorkerSuite) apiOpen(info *api.Info, opts api.DialOpts) (api.Connection, error) {
	s.stub.MethodCall(s, "APIOpen", info, opts)
	return s.apiConn, s.stub.NextErr()
}

func (s *WorkerSuite) requestVote(t raft.Transport, tag string) (raft.RequestVoteResponse, error) {
	var resp raft.RequestVoteResponse
	req := &raft.RequestVoteRequest{}
	return resp, t.RequestVote(raft.ServerID(tag), raft.ServerAddress(tag), req, &resp)
}

func (s *WorkerSuite) TestStartStop(c *gc.C) {
	workertest.CleanKill(c, s.worker)
}

func (s *WorkerSuite) TestLocalAddr(c *gc.C) {
	addr := s.worker.LocalAddr()
	c.Assert(addr, gc.Equals, raft.ServerAddress("machine-123"))
}

func (s *WorkerSuite) TestInvalidAddr(c *gc.C) {
	_, err := s.requestVote(s.worker, "zoink123")
	c.Assert(err, gc.ErrorMatches, `dial failed: "zoink123" is not a valid tag`)
	c.Assert(errors.Cause(err), jc.DeepEquals, net.InvalidAddrError(`"zoink123" is not a valid tag`))
}

func (s *WorkerSuite) TestTransportWorkerStopped(c *gc.C) {
	workertest.CleanKill(c, s.worker)

	_, err := s.requestVote(s.worker, "machine-123")
	c.Assert(err, gc.ErrorMatches, "dial failed: worker stopped")

	c.Assert(errors.Cause(err), gc.Implements, new(net.Error))
	netErr := errors.Cause(err).(net.Error)
	c.Assert(netErr.Temporary(), jc.IsTrue)
	c.Assert(netErr.Timeout(), jc.IsFalse)
}

func (s *WorkerSuite) TestTransportTimeout(c *gc.C) {
	config := s.config
	config.Timeout = time.Millisecond
	worker := s.newWorker(c, config)

	_, err := s.requestVote(worker, "machine-123")
	c.Assert(err, gc.ErrorMatches, "dial failed: timed out dialing")

	c.Assert(errors.Cause(err), gc.Implements, new(net.Error))
	netErr := errors.Cause(err).(net.Error)
	c.Assert(netErr.Temporary(), jc.IsTrue)
	c.Assert(netErr.Timeout(), jc.IsTrue)
}

func (s *WorkerSuite) TestAPIOpen(c *gc.C) {
	s.apiConn.SetErrors(errors.New("DialConn failed"))

	_, err := s.requestVote(s.worker, "machine-123")
	c.Assert(err, gc.ErrorMatches, "dial failed: DialConn failed")

	s.stub.CheckCallNames(c, "APIOpen")
	args := s.stub.Calls()[0].Args
	c.Assert(args, gc.HasLen, 2)

	c.Assert(args[0], jc.DeepEquals, &api.Info{
		Addrs:     []string{"testing.invalid:1234"},
		SkipLogin: true,
	})
}

func (s *WorkerSuite) TestRoundTrip(c *gc.C) {
	go func() {
		rpc := <-s.worker.Consumer()
		resp := &raft.RequestVoteResponse{
			Granted: true,
		}
		rpc.Respond(resp, nil)
	}()
	resp, err := s.requestVote(s.worker, "machine-123")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.Granted, jc.IsTrue)
}
