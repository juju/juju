// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rpc_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/jsoncodec"
	"github.com/juju/juju/testing"
)

type dispatchSuite struct {
	testing.BaseSuite

	server     *httptest.Server
	serverAddr string

	dead   chan error
	unique int64
}

var _ = gc.Suite(&dispatchSuite{})

func (s *dispatchSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	loggo.GetLogger("juju.rpc").SetLogLevel(loggo.TRACE)

	s.dead = make(chan error)

	rpcServer := func(ws *websocket.Conn) {
		codec := jsoncodec.NewWebsocket(ws)
		conn := rpc.NewConn(codec, nil)

		conn.Serve(&DispatchRoot{}, nil, nil)
		conn.Start(context.Background())

		<-conn.Dead()
		s.dead <- conn.Close()
	}

	unique := atomic.AddInt64(&s.unique, 1)

	http.Handle(fmt.Sprintf("/rpc%d", unique), websocketHandler(rpcServer))

	s.server = httptest.NewServer(nil)
	s.serverAddr = s.server.Listener.Addr().String()

	s.AddCleanup(func(*gc.C) {
		close(s.dead)
		s.server.Close()
	})
}

var wsUpgrader = &websocket.Upgrader{
	CheckOrigin: func(*http.Request) bool {
		return true
	},
}

func websocketHandler(f func(*websocket.Conn)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		c, err := wsUpgrader.Upgrade(w, req, nil)
		if err == nil {
			f(c)
		}
	})
}

func (s *dispatchSuite) TestWSWithoutParamsV0(c *gc.C) {
	err := s.requestV0(c, `{"RequestId":1,"Type": "DispatchDummy","Id": "without","Request":"DoSomething"}`)
	c.Assert(errors.Is(err, errors.NotSupported), jc.IsTrue)
}

func (s *dispatchSuite) TestWSWithParamsV0(c *gc.C) {
	err := s.requestV0(c, `{"RequestId":2,"Type": "DispatchDummy","Id": "with","Request":"DoSomething", "Params": {}}`)
	c.Assert(errors.Is(err, errors.NotSupported), jc.IsTrue)
}

func (s *dispatchSuite) TestWSWithoutParamsV1(c *gc.C) {
	resp := s.requestV1(c, `{"request-id":1,"type": "DispatchDummy","id": "without","request":"DoSomething"}`)
	s.assertResponse(c, resp, `{"request-id":1,"response":{}}`)
	s.ensureFinish(c)
}

func (s *dispatchSuite) TestWSWithParamsV1(c *gc.C) {
	resp := s.requestV1(c, `{"request-id":2,"type": "DispatchDummy","id": "with","request":"DoSomething", "params": {}}`)
	s.assertResponse(c, resp, `{"request-id":2,"response":{}}`)
	s.ensureFinish(c)
}

func (s *dispatchSuite) assertResponse(c *gc.C, obtained, expected string) {
	c.Assert(obtained, gc.Equals, expected+"\n")
}

func (s *dispatchSuite) ensureFinish(c *gc.C) {
	// Ensure the test is finished, before starting another one. This is to
	// prevent a data race on the dead channel in SetupTest.
	select {
	case <-s.dead:
	case <-time.After(testing.LongWait):
		c.Fatalf("timeout waiting for response")
	}
}

// request performs one request to the test server via websockets.
func (s *dispatchSuite) requestV0(c *gc.C, req string) error {
	ws := s.request(c, req)

	result := make(chan struct{})

	go func() {
		_, _, err := ws.ReadMessage()
		c.Check(err, gc.NotNil)

		close(result)
	}()

	select {
	case err := <-s.dead:
		return err
	case <-result:
		return nil
	case <-time.After(testing.LongWait):
		c.Fatalf("timeout waiting for response")
		return nil
	}
}

// request performs one request to the test server via websockets.
func (s *dispatchSuite) requestV1(c *gc.C, req string) string {
	ws := s.request(c, req)

	result := make(chan string)

	go func() {
		_, resp, err := ws.ReadMessage()
		c.Check(err, jc.ErrorIsNil)

		err = ws.Close()
		c.Assert(err, jc.ErrorIsNil)

		result <- string(resp)
	}()

	select {
	case err := <-s.dead:
		c.Assert(err, jc.ErrorIsNil)
		return ""
	case resp := <-result:
		return resp
	case <-time.After(testing.LongWait):
		c.Fatalf("timeout waiting for response")
		return ""
	}
}

func (s *dispatchSuite) request(c *gc.C, req string) *websocket.Conn {
	url := fmt.Sprintf("ws://%s/rpc%d", s.serverAddr, atomic.LoadInt64(&s.unique))
	ws, _, err := websocket.DefaultDialer.Dial(url, http.Header{
		"Origin": {"http://localhost"},
	})
	c.Assert(err, jc.ErrorIsNil)

	reqData := []byte(req)
	err = ws.WriteMessage(websocket.TextMessage, reqData)
	c.Assert(err, jc.ErrorIsNil)

	return ws
}

// DispatchRoot simulates the root for the test.
type DispatchRoot struct{}

func (*DispatchRoot) DispatchDummy(id string) (*DispatchDummy, error) {
	return &DispatchDummy{}, nil
}

// DispatchDummy is the type to whish the request is dispatched.
type DispatchDummy struct{}

func (d *DispatchDummy) DoSomething() {}
