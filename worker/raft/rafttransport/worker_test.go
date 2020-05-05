// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rafttransport_test

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/hashicorp/raft"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/httpcontext"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/pubsub/apiserver"
	"github.com/juju/juju/pubsub/centralhub"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/raft/rafttransport"
)

var controllerTag = names.NewMachineTag("123")

type workerFixture struct {
	testing.IsolationSuite
	config   rafttransport.Config
	authInfo httpcontext.AuthInfo
	clock    *testclock.Clock
}

func (s *workerFixture) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	tag := names.NewMachineTag("123")
	s.clock = testclock.NewClock(time.Time{})
	s.config = rafttransport.Config{
		APIInfo: &api.Info{
			Tag:      tag,
			Password: "valid-password",
			Addrs:    []string{"testing.invalid:1234"},
		},
		DialConn:      rafttransport.DialConn,
		Hub:           centralhub.New(tag),
		Mux:           apiserverhttp.NewMux(),
		Authenticator: &mockAuthenticator{auth: s.auth},
		Path:          "/raft/path",
		LocalID:       "123",
		Timeout:       coretesting.LongWait,
		TLSConfig:     &tls.Config{},
		Clock:         s.clock,
	}

	logger := loggo.GetLogger("juju.worker.raft")
	oldLevel := logger.LogLevel()
	logger.SetLogLevel(loggo.TRACE)
	s.AddCleanup(func(c *gc.C) {
		logger.SetLogLevel(oldLevel)
	})
}

func (s *workerFixture) auth(req *http.Request) (httpcontext.AuthInfo, error) {
	user, pass, ok := req.BasicAuth()
	if !ok || pass != "valid-password" {
		return httpcontext.AuthInfo{}, errors.Unauthorizedf("request")
	}
	tag, err := names.ParseTag(user)
	if err != nil {
		return httpcontext.AuthInfo{}, errors.Trace(err)
	}
	return httpcontext.AuthInfo{
		Entity:     &mockEntity{tag: tag},
		Controller: tag == controllerTag,
	}, nil
}

type WorkerValidationSuite struct {
	workerFixture
}

var _ = gc.Suite(&WorkerValidationSuite{})

func (s *WorkerValidationSuite) TestValidate(c *gc.C) {
	w, err := rafttransport.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	workertest.DirtyKill(c, w)
}

func (s *WorkerValidationSuite) TestValidateErrors(c *gc.C) {
	type test struct {
		f      func(*rafttransport.Config)
		expect string
	}
	tests := []test{{
		func(cfg *rafttransport.Config) { cfg.APIInfo = nil },
		"nil APIInfo not valid",
	}, {
		func(cfg *rafttransport.Config) { cfg.DialConn = nil },
		"nil DialConn not valid",
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
		func(cfg *rafttransport.Config) { cfg.LocalID = "" },
		"empty LocalID not valid",
	}, {
		func(cfg *rafttransport.Config) { cfg.TLSConfig = nil },
		"nil TLSConfig not valid",
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
	server *httptest.Server
	worker *rafttransport.Worker
	reqs   chan apiserver.DetailsRequest
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) SetUpTest(c *gc.C) {
	s.workerFixture.SetUpTest(c)

	s.server = httptest.NewTLSServer(s.config.Mux)
	s.AddCleanup(func(c *gc.C) {
		s.server.Close()
	})
	clientTransport := s.server.Client().Transport.(*http.Transport)
	s.config.TLSConfig = clientTransport.TLSClientConfig

	s.reqs = make(chan apiserver.DetailsRequest, 10)
	unsubscribe, _ := s.config.Hub.Subscribe(apiserver.DetailsRequestTopic,
		func(reqs chan apiserver.DetailsRequest) func(_ string, req apiserver.DetailsRequest, err error) {
			return func(_ string, req apiserver.DetailsRequest, err error) {
				c.Check(err, jc.ErrorIsNil)
				reqs <- req
			}
		}(s.reqs),
	)
	s.AddCleanup(func(c *gc.C) {
		unsubscribe()
	})

	s.worker = s.newWorker(c, s.config)
	s.config.Hub.Publish(apiserver.DetailsTopic, apiserver.Details{
		Servers: map[string]apiserver.APIServer{
			"123": {
				ID:              "123",
				InternalAddress: "testing.invalid:1234",
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
	return worker.(*rafttransport.Worker)
}

func (s *WorkerSuite) requestVote(t raft.Transport) (raft.RequestVoteResponse, error) {
	var resp raft.RequestVoteResponse
	req := &raft.RequestVoteRequest{}
	serverID := raft.ServerID("123")
	serverAddress := raft.ServerAddress(s.server.Listener.Addr().String())
	return resp, t.RequestVote(serverID, serverAddress, req, &resp)
}

func (s *WorkerSuite) TestStartStop(c *gc.C) {
	workertest.CleanKill(c, s.worker)
}

func (s *WorkerSuite) TestLocalAddr(c *gc.C) {
	addr := s.worker.LocalAddr()
	c.Assert(addr, gc.Equals, raft.ServerAddress("testing.invalid:1234"))

	// Publishing an address change should lead to the transport
	// advertising the new address eventually.
	newAddress := "testing.invalid:5678"
	s.config.Hub.Publish(apiserver.DetailsTopic, apiserver.Details{
		Servers: map[string]apiserver.APIServer{
			"123": {
				ID:              "123",
				InternalAddress: newAddress,
			},
		},
	})
	for a := coretesting.LongAttempt.Start(); a.HasNext(); {
		addr = s.worker.LocalAddr()
		if addr == raft.ServerAddress(newAddress) {
			return
		}
	}
	c.Fatalf(
		"waited %s for address to change to %s, got %s",
		coretesting.LongAttempt.Total, newAddress, addr,
	)
}

func (s *WorkerSuite) TestTransportWorkerStopped(c *gc.C) {
	workertest.CleanKill(c, s.worker)

	_, err := s.requestVote(s.worker)
	c.Assert(err, gc.ErrorMatches, "dial failed: worker stopped")

	c.Assert(errors.Cause(err), gc.Implements, new(net.Error))
	netErr := errors.Cause(err).(net.Error)
	c.Assert(netErr.Temporary(), jc.IsTrue)
	c.Assert(netErr.Timeout(), jc.IsFalse)
}

func (s *WorkerSuite) TestTransportTimeout(c *gc.C) {
	config := s.config
	config.Timeout = time.Nanosecond
	worker := s.newWorker(c, config)

	// Instead of using the test server, set up a simple listener with no
	// handling. This will always cause a connection timeout.
	noAcceptListener, err := net.Listen("tcp", ":0")
	c.Assert(err, jc.ErrorIsNil)
	var resp raft.RequestVoteResponse
	req := &raft.RequestVoteRequest{}
	serverID := raft.ServerID("123")
	serverAddress := raft.ServerAddress(noAcceptListener.Addr().String())
	_, err = resp, worker.RequestVote(serverID, serverAddress, req, &resp)

	c.Assert(err, gc.ErrorMatches, "dial failed:.*timed out.*")
	c.Assert(errors.Cause(err), gc.Implements, new(net.Error))
	netErr := errors.Cause(err).(net.Error)
	c.Assert(netErr.Temporary(), jc.IsTrue)
	c.Assert(netErr.Timeout(), jc.IsTrue)
}

func (s *WorkerSuite) TestRoundTrip(c *gc.C) {
	go func() {
		rpc := <-s.worker.Consumer()
		resp := &raft.RequestVoteResponse{
			Granted: true,
		}
		rpc.Respond(resp, nil)
	}()
	resp, err := s.requestVote(s.worker)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.Granted, jc.IsTrue)
}

func (s *WorkerSuite) TestRequestsDetails(c *gc.C) {
	// The worker gets started in SetUpTest.
	select {
	case req := <-s.reqs:
		c.Assert(req, gc.Equals, apiserver.DetailsRequest{
			Requester: "raft-transport-stream-layer",
			LocalOnly: true,
		})
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for details request")
	}
}

type WorkerTimeoutSuite struct {
	workerFixture
	server *httptest.Server
}

var _ = gc.Suite(&WorkerTimeoutSuite{})

func (s *WorkerTimeoutSuite) SetUpTest(c *gc.C) {
	s.workerFixture.SetUpTest(c)

	s.server = httptest.NewTLSServer(s.config.Mux)
	s.AddCleanup(func(c *gc.C) {
		s.server.Close()
	})
	clientTransport := s.server.Client().Transport.(*http.Transport)
	s.config.TLSConfig = clientTransport.TLSClientConfig
}

func (s *WorkerTimeoutSuite) TestLocalAddrTimeout(c *gc.C) {
	// To prevent workers that use raft-transport from hanging, we need to make LocalAddr time out.
	w, err := rafttransport.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) {
		workertest.DirtyKill(c, w)
	})
	worker := w.(*rafttransport.Worker)

	resChan := make(chan raft.ServerAddress)
	go func() {
		resChan <- worker.LocalAddr()
	}()

	// We never publish an apiserver address.
	s.clock.WaitAdvance(2*rafttransport.AddrTimeout, coretesting.LongWait, 1)

	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for address timeout")
	case res := <-resChan:
		c.Assert(res, gc.Equals, raft.ServerAddress("address.invalid:0"))
	}
}

type mockEntity struct {
	tag names.Tag
}

func (e *mockEntity) Tag() names.Tag {
	return e.tag
}

type mockAuthenticator struct {
	auth func(*http.Request) (httpcontext.AuthInfo, error)
}

func (a *mockAuthenticator) Authenticate(req *http.Request) (httpcontext.AuthInfo, error) {
	return a.auth(req)
}

func (a *mockAuthenticator) AuthenticateLoginRequest(
	ctx context.Context,
	serverHost string,
	modelUUID string,
	req params.LoginRequest,
) (httpcontext.AuthInfo, error) {
	return httpcontext.AuthInfo{}, errors.New("blah")
}
