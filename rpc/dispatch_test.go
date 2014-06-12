// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rpc_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"

	"code.google.com/p/go.net/websocket"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/jsoncodec"
)

type dispatchSuite struct {
}

var (
	_          = gc.Suite(&dispatchSuite{})
	serverAddr string
	once       sync.Once
)

type ReflectDummy struct{}

func (*ReflectDummy) DoSomething() {}

func (*dispatchSuite) TestWebsocket(c *gc.C) {
	once.Do(startWebsocketServer)

	respA := request(c, `{"RequestId":1,"Type": "ReflectDummy","Id": "id","Request":"DoSomething", "Params": {}}`)
	c.Assert(respA, gc.Equals, "OK")

	respB := request(c, `{"RequestId":1,"Type": "ReflectDummy","Id": "id","Request":"DoSomething"}`)
	c.Assert(respB, gc.Equals, "OK")
}

func request(c *gc.C, req string) string {
	url := fmt.Sprintf("ws://%s/jsoncodec", serverAddr)
	ws, err := websocket.Dial(url, "", "http://localhost")
	c.Assert(err, gc.IsNil)

	reqdata := []byte(req)
	_, err = ws.Write(reqdata)
	c.Assert(err, gc.IsNil)

	var resp = make([]byte, 512)
	n, err := ws.Read(resp)
	c.Assert(err, gc.IsNil)
	resp = resp[0:n]

	err = ws.Close()
	c.Assert(err, gc.IsNil)

	return string(resp)
}

func startWebsocketServer() {
	codecServer := func(ws *websocket.Conn) {
		codec := jsoncodec.NewWebsocket(ws)
		conn := rpc.NewConn(codec, nil)
		conn.Start()
	}
	http.Handle("/jsoncodec", websocket.Handler(codecServer))
	server := httptest.NewServer(nil)
	serverAddr = server.Listener.Addr().String()
}
