// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"net"
	"sync/atomic"
	"time"

	"github.com/juju/errors"
	"github.com/juju/retry"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	"github.com/juju/utils/parallel"
	"golang.org/x/net/websocket"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jjtesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/rpc"
	jtesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

type apiclientSuite struct {
	jjtesting.JujuConnSuite
}

var _ = gc.Suite(&apiclientSuite{})

func (s *apiclientSuite) TestConnectWebsocketToEnv(c *gc.C) {
	info := s.APIInfo(c)
	conn, _, err := api.ConnectWebsocket(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	defer conn.Close()
	assertConnAddrForEnv(c, conn, info.Addrs[0], s.State.ModelUUID(), "/api")
}

func (s *apiclientSuite) TestConnectWebsocketToRoot(c *gc.C) {
	info := s.APIInfo(c)
	info.ModelTag = names.NewModelTag("")
	conn, _, err := api.ConnectWebsocket(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	defer conn.Close()
	assertConnAddrForRoot(c, conn, info.Addrs[0])
}

func (s *apiclientSuite) TestConnectWebsocketMultiple(c *gc.C) {
	// Create a socket that proxies to the API server.
	info := s.APIInfo(c)
	serverAddr := info.Addrs[0]
	proxy := testing.NewTCPProxy(c, serverAddr)
	defer proxy.Close()

	// Check that we can use the proxy to connect.
	info.Addrs = []string{proxy.Addr()}
	conn, _, err := api.ConnectWebsocket(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	conn.Close()
	assertConnAddrForEnv(c, conn, proxy.Addr(), s.State.ModelUUID(), "/api")

	// Now break Addrs[0], and ensure that Addrs[1]
	// is successfully connected to.
	proxy.Close()
	info.Addrs = []string{proxy.Addr(), serverAddr}
	conn, _, err = api.ConnectWebsocket(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	conn.Close()
	assertConnAddrForEnv(c, conn, serverAddr, s.State.ModelUUID(), "/api")
}

func (s *apiclientSuite) TestConnectWebsocketMultipleError(c *gc.C) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, jc.ErrorIsNil)
	defer listener.Close()
	// count holds the number of times we've accepted a connection.
	var count int32
	go func() {
		for {
			client, err := listener.Accept()
			if err != nil {
				return
			}
			atomic.AddInt32(&count, 1)
			client.Close()
		}
	}()
	info := s.APIInfo(c)
	addr := listener.Addr().String()
	info.Addrs = []string{addr, addr, addr}
	_, _, err = api.ConnectWebsocket(info, api.DialOpts{})
	c.Assert(err, gc.ErrorMatches, `unable to connect to API: websocket.Dial wss://.*/model/[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}/api: .*`)
	c.Assert(atomic.LoadInt32(&count), gc.Equals, int32(3))
}

func (s *apiclientSuite) TestOpen(c *gc.C) {
	info := s.APIInfo(c)
	st, err := api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	c.Assert(st.Addr(), gc.Equals, info.Addrs[0])
	modelTag, ok := st.ModelTag()
	c.Assert(ok, jc.IsTrue)
	c.Assert(modelTag, gc.Equals, s.State.ModelTag())

	remoteVersion, versionSet := st.ServerVersion()
	c.Assert(versionSet, jc.IsTrue)
	c.Assert(remoteVersion, gc.Equals, jujuversion.Current)
}

func (s *apiclientSuite) TestOpenHonorsModelTag(c *gc.C) {
	info := s.APIInfo(c)

	// TODO(jam): 2014-06-05 http://pad.lv/1326802
	// we want to test this eventually, but for now s.APIInfo uses
	// conn.StateInfo() which doesn't know about ModelTag.
	// c.Check(info.ModelTag, gc.Equals, env.Tag())
	// c.Assert(info.ModelTag, gc.Not(gc.Equals), "")

	// We start by ensuring we have an invalid tag, and Open should fail.
	info.ModelTag = names.NewModelTag("bad-tag")
	_, err := api.Open(info, api.DialOpts{})
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: `unknown model: "bad-tag"`,
		Code:    "model not found",
	})
	c.Check(params.ErrCode(err), gc.Equals, params.CodeModelNotFound)

	// Now set it to the right tag, and we should succeed.
	info.ModelTag = s.State.ModelTag()
	st, err := api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	st.Close()

	// Backwards compatibility, we should succeed if we do not set an
	// model tag
	info.ModelTag = names.NewModelTag("")
	st, err = api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	st.Close()
}

func (s *apiclientSuite) TestServerRoot(c *gc.C) {
	url := api.ServerRoot(s.APIState.Client())
	c.Assert(url, gc.Matches, "https://localhost:[0-9]+")
}

func (s *apiclientSuite) TestDialWebsocketStopped(c *gc.C) {
	stopped := make(chan struct{})
	f := api.NewWebsocketDialer(nil, api.DialOpts{})
	close(stopped)
	result, err := f(stopped)
	c.Assert(err, gc.Equals, parallel.ErrStopped)
	c.Assert(result, gc.IsNil)
}

func (s *apiclientSuite) TestOpenWithNoCACert(c *gc.C) {
	// This is hard to test as we have no way of affecting the system roots,
	// so instead we check that the error that we get implies that
	// we're using the system roots.

	info := s.APIInfo(c)
	info.CACert = ""

	t0 := time.Now()
	// Use a long timeout so that we can check that the retry
	// logic doesn't retry.
	_, err := api.Open(info, api.DialOpts{
		Timeout:    20 * time.Second,
		RetryDelay: 2 * time.Second,
	})
	c.Assert(err, gc.ErrorMatches, `unable to connect to API: websocket.Dial wss://.*/api: x509: certificate signed by unknown authority`)

	if time.Since(t0) > 5*time.Second {
		c.Errorf("looks like API is retrying on connection when there is an X509 error")
	}
}

func (s *apiclientSuite) TestOpenWithRedirect(c *gc.C) {
	redirectToHosts := []string{"0.1.2.3:1234", "0.1.2.4:1235"}
	redirectToCACert := "fake CA cert"

	srv := apiservertesting.NewAPIServer(func(modelUUID string) interface{} {
		return &redirectAPI{
			modelUUID:        modelUUID,
			redirectToHosts:  redirectToHosts,
			redirectToCACert: redirectToCACert,
		}
	})
	defer srv.Close()

	_, err := api.Open(&api.Info{
		Addrs:    srv.Addrs,
		CACert:   jtesting.CACert,
		ModelTag: names.NewModelTag("beef1beef1-0000-0000-000011112222"),
	}, api.DialOpts{})
	c.Assert(err, gc.ErrorMatches, `redirection to alternative server required`)

	hps, _ := network.ParseHostPorts(redirectToHosts...)
	c.Assert(errors.Cause(err), jc.DeepEquals, &api.RedirectError{
		Servers: [][]network.HostPort{hps},
		CACert:  redirectToCACert,
	})
}

func (s *apiclientSuite) TestAPICallNoError(c *gc.C) {
	clock := &fakeClock{}
	conn := api.NewTestingState(api.TestingStateParams{
		RPCConnection: &fakeRPCConnection{},
		Clock:         clock,
	})

	err := conn.APICall("facade", 1, "id", "method", nil, nil)
	c.Check(err, jc.ErrorIsNil)
	c.Check(clock.waits, gc.HasLen, 0)
}

func (s *apiclientSuite) TestAPICallError(c *gc.C) {
	clock := &fakeClock{}
	conn := api.NewTestingState(api.TestingStateParams{
		RPCConnection: &fakeRPCConnection{
			errors: []error{errors.BadRequestf("boom")},
		},
		Clock: clock,
	})

	err := conn.APICall("facade", 1, "id", "method", nil, nil)
	c.Check(err.Error(), gc.Equals, "boom")
	c.Check(err, jc.Satisfies, errors.IsBadRequest)
	c.Check(clock.waits, gc.HasLen, 0)
}

func (s *apiclientSuite) TestAPICallRetries(c *gc.C) {
	clock := &fakeClock{}
	conn := api.NewTestingState(api.TestingStateParams{
		RPCConnection: &fakeRPCConnection{
			errors: []error{
				errors.Trace(
					&rpc.RequestError{
						Message: "hmm...",
						Code:    params.CodeRetry,
					}),
			},
		},
		Clock: clock,
	})

	err := conn.APICall("facade", 1, "id", "method", nil, nil)
	c.Check(err, jc.ErrorIsNil)
	c.Check(clock.waits, jc.DeepEquals, []time.Duration{100 * time.Millisecond})
}

func (s *apiclientSuite) TestAPICallRetriesLimit(c *gc.C) {
	clock := &fakeClock{}
	retryError := errors.Trace(&rpc.RequestError{Message: "hmm...", Code: params.CodeRetry})
	var errors []error
	for i := 0; i < 10; i++ {
		errors = append(errors, retryError)
	}
	conn := api.NewTestingState(api.TestingStateParams{
		RPCConnection: &fakeRPCConnection{
			errors: errors,
		},
		Clock: clock,
	})

	err := conn.APICall("facade", 1, "id", "method", nil, nil)
	c.Check(err, jc.Satisfies, retry.IsDurationExceeded)
	c.Check(err, gc.ErrorMatches, `.*hmm... \(retry\)`)
	c.Check(clock.waits, jc.DeepEquals, []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		400 * time.Millisecond,
		800 * time.Millisecond,
		1500 * time.Millisecond,
		1500 * time.Millisecond,
		1500 * time.Millisecond,
		1500 * time.Millisecond,
		1500 * time.Millisecond,
	})
}

type fakeClock struct {
	clock.Clock

	now   time.Time
	waits []time.Duration
}

func (f *fakeClock) Now() time.Time {
	if f.now.IsZero() {
		f.now = time.Now()
	}
	return f.now
}

func (f *fakeClock) After(d time.Duration) <-chan time.Time {
	f.waits = append(f.waits, d)
	f.now = f.now.Add(d)
	return time.After(0)
}

type fakeRPCConnection struct {
	pos    int
	errors []error
}

func (f *fakeRPCConnection) Close() error {
	return nil
}

func (f *fakeRPCConnection) Call(req rpc.Request, params, response interface{}) error {
	if f.pos >= len(f.errors) {
		return nil
	}
	err := f.errors[f.pos]
	f.pos++
	return err
}

type redirectAPI struct {
	redirected       bool
	modelUUID        string
	redirectToHosts  []string
	redirectToCACert string
}

func (r *redirectAPI) Admin(id string) (*redirectAPIAdmin, error) {
	return &redirectAPIAdmin{r}, nil
}

type redirectAPIAdmin struct {
	r *redirectAPI
}

func (a *redirectAPIAdmin) Login(req params.LoginRequest) (params.LoginResult, error) {
	if a.r.modelUUID != "beef1beef1-0000-0000-000011112222" {
		return params.LoginResult{}, errors.New("logged into unexpected model")
	}
	a.r.redirected = true
	return params.LoginResult{}, params.Error{
		Message: "redirect",
		Code:    params.CodeRedirect,
	}
}

func (a *redirectAPIAdmin) RedirectInfo() (params.RedirectInfoResult, error) {
	if !a.r.redirected {
		return params.RedirectInfoResult{}, errors.New("not redirected")
	}
	hps, err := network.ParseHostPorts(a.r.redirectToHosts...)
	if err != nil {
		panic(err)
	}
	return params.RedirectInfoResult{
		Servers: [][]params.HostPort{params.FromNetworkHostPorts(hps)},
		CACert:  a.r.redirectToCACert,
	}, nil
}

func assertConnAddrForEnv(c *gc.C, conn *websocket.Conn, addr, modelUUID, tail string) {
	c.Assert(conn.RemoteAddr(), gc.Matches, "^wss://"+addr+"/model/"+modelUUID+tail+"$")
}

func assertConnAddrForRoot(c *gc.C, conn *websocket.Conn, addr string) {
	c.Assert(conn.RemoteAddr(), gc.Matches, "^wss://"+addr+"/api$")
}
