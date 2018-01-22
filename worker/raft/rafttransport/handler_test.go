// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rafttransport_test

import (
	"crypto/tls"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"

	"github.com/hashicorp/raft"
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/apiserverhttp"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/raft/rafttransport"
)

type HandlerSuite struct {
	testing.IsolationSuite
	connections chan net.Conn
	handler     *rafttransport.Handler
	server      *httptest.Server
}

var _ = gc.Suite(&HandlerSuite{})

func (s *HandlerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.connections = make(chan net.Conn)
	s.handler = rafttransport.NewHandler(s.connections, nil)
	s.server = httptest.NewTLSServer(s.handler)
	s.AddCleanup(func(c *gc.C) {
		s.server.Close()
	})
}

func (s *HandlerSuite) TestHandler(c *gc.C) {
	u, err := url.Parse(s.server.URL)
	c.Assert(err, jc.ErrorIsNil)
	dialRaw := func(addr raft.ServerAddress, timeout time.Duration) (net.Conn, error) {
		tlsConfig := s.server.Client().Transport.(*http.Transport).TLSClientConfig
		return tls.Dial("tcp", u.Host, tlsConfig)
	}
	dialer := rafttransport.Dialer{
		APIInfo: &api.Info{},
		Path:    "/raft",
		DialRaw: dialRaw,
	}
	clientConn, err := dialer.Dial("", 0)
	c.Assert(err, jc.ErrorIsNil)
	defer clientConn.Close()

	var serverConn net.Conn
	select {
	case conn := <-s.connections:
		serverConn = conn
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for server connection")
	}
	defer serverConn.Close()

	payload := "hello, server!"
	n, err := clientConn.Write([]byte(payload))
	c.Assert(err, jc.ErrorIsNil)

	read := make([]byte, n)
	n, err = serverConn.Read(read)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(n, gc.Equals, len(payload))
	c.Assert(string(read), gc.Equals, payload)
}

type ControllerHandlerSuite struct {
	testing.IsolationSuite
	stub     testing.Stub
	mux      *apiserverhttp.Mux
	authInfo apiserverhttp.AuthInfo
	handler  *rafttransport.ControllerHandler
	server   *httptest.Server
}

var _ = gc.Suite(&ControllerHandlerSuite{})

func (s *ControllerHandlerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub.ResetCalls()
	s.authInfo = apiserverhttp.AuthInfo{
		Tag:        names.NewMachineTag("99"),
		Controller: true,
	}
	s.handler = &rafttransport.ControllerHandler{
		Mux: s.mux,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, "hello!")
		}),
	}
	s.mux = apiserverhttp.NewMux(apiserverhttp.WithAuth(func(req *http.Request) (apiserverhttp.AuthInfo, error) {
		s.stub.AddCall("Auth", req)
		return s.authInfo, s.stub.NextErr()
	}))
	s.mux.AddHandler("GET", "/auth", s.handler)

	mux := http.NewServeMux()
	mux.Handle("/auth", s.mux)
	mux.Handle("/noauth", s.handler)
	s.server = httptest.NewTLSServer(mux)
	s.AddCleanup(func(c *gc.C) {
		s.server.Close()
	})
}

func (s *ControllerHandlerSuite) TestControllerHandler(c *gc.C) {
	client := s.server.Client()
	resp, err := client.Get(s.server.URL + "/auth")
	c.Assert(err, jc.ErrorIsNil)
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, "hello!")
}

func (s *ControllerHandlerSuite) TestControllerHandlerNoAuthHandler(c *gc.C) {
	client := s.server.Client()
	resp, err := client.Get(s.server.URL + "/noauth")
	c.Assert(err, jc.ErrorIsNil)
	defer resp.Body.Close()

	// /noauth points to the ControllerHandler, but without going through
	// the required apiserverhttp.Mux. This is a programming error.
	data, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.StatusCode, gc.Equals, http.StatusInternalServerError)
	c.Assert(string(data), gc.Equals, "no authentication handler found\n")
}

func (s *ControllerHandlerSuite) TestControllerHandlerAuthFailure(c *gc.C) {
	s.stub.SetErrors(errors.NewUnauthorized(nil, "i say nay sir"))

	client := s.server.Client()
	resp, err := client.Get(s.server.URL + "/auth")
	c.Assert(err, jc.ErrorIsNil)
	defer resp.Body.Close()

	c.Assert(resp.StatusCode, gc.Equals, http.StatusUnauthorized)
	c.Assert(resp.Header.Get("WWW-Authenticate"), gc.Equals, `Basic realm="juju"`)

	data, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, "i say nay sir\n")
}

func (s *ControllerHandlerSuite) TestControllerHandlerNonController(c *gc.C) {
	s.authInfo.Controller = false

	client := s.server.Client()
	resp, err := client.Get(s.server.URL + "/auth")
	c.Assert(err, jc.ErrorIsNil)
	defer resp.Body.Close()

	c.Assert(resp.StatusCode, gc.Equals, http.StatusForbidden)
	data, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, "controller agents only\n")
}
