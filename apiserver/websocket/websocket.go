// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package websocket

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
)

var logger = loggo.GetLogger("juju.apiserver.websocket")

// Use a 64k frame size for the websockets while we need to deal
// with x/net/websocket connections that don't deal with receiving
// fragmented messages.
const websocketFrameSize = 65536

const (
	// PongDelay is how long the server will wait for a pong to be sent
	// before the websocket is considered broken.
	PongDelay = 90 * time.Second

	// PingPeriod is how often ping messages are sent. This should be shorter
	// than the pongDelay, but not by too much. The difference here allows
	// the remote endpoint 30 seconds to respond to the ping as a ping is sent
	// every 60s, and when a pong is received the read deadline is advanced
	// another 90s.
	PingPeriod = 60 * time.Second

	// WriteWait is how long the write call can take before it errors out.
	WriteWait = 10 * time.Second
)

var websocketUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
	// In order to deal with the remote side not handling message
	// fragmentation, we default to largeish frames.
	ReadBufferSize:  websocketFrameSize,
	WriteBufferSize: websocketFrameSize,
}

// Conn wraps a gorilla/websocket.Conn, providing additional Juju-specific
// functionality.
type Conn struct {
	*websocket.Conn
}

// Serve upgrades an HTTP connection to a websocket, and
// serves the given handler.
func Serve(w http.ResponseWriter, req *http.Request, handler func(ws *Conn)) {
	conn, err := websocketUpgrader.Upgrade(w, req, nil)
	if err != nil {
		logger.Errorf("problem initiating websocket: %v", err)
		return
	}
	handler(&Conn{conn})
}

// SendInitialErrorV0 writes out the error as a params.ErrorResult serialized
// with JSON with a new line character at the end.
//
// This is a hangover from the initial debug-log streaming endpoint where the
// client read the first line, and then just got a stream of data. We should
// look to version the streaming endpoints to get rid of the trailing newline
// character for message based connections, which is all of them now.
func (conn *Conn) SendInitialErrorV0(err error) error {
	wrapped := &params.ErrorResult{
		Error: common.ServerError(err),
	}

	body, err := json.Marshal(wrapped)
	if err != nil {
		errors.Annotatef(err, "cannot marshal error %#v", wrapped)
		return err
	}
	body = append(body, '\n')

	writer, err := conn.NextWriter(websocket.TextMessage)
	if err != nil {
		return errors.Annotate(err, "problem getting writer")
	}
	defer writer.Close()
	_, err = writer.Write(body)

	if wrapped.Error != nil {
		// Tell the other end we are closing.
		conn.WriteMessage(websocket.CloseMessage, []byte{})
	}

	return errors.Trace(err)
}
