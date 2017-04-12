// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/juju/errors"
	"github.com/juju/utils/featureflag"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/state"
)

// Hub defines the publish method that the handler uses to publish
// messages on the centralhub of the apiserver.
type Hub interface {
	Publish(string, interface{}) (<-chan struct{}, error)
}

func newPubSubHandler(h httpContext, hub Hub) http.Handler {
	return &pubsubHandler{
		ctxt: h,
		hub:  hub,
	}
}

type pubsubHandler struct {
	ctxt httpContext
	hub  Hub
}

func (h *pubsubHandler) authenticate(req *http.Request) error {
	// We authenticate against the controller state instance that is held
	// by Server.
	_, releaser, entity, err := h.ctxt.stateForRequestAuthenticated(req)
	if err != nil {
		return errors.Trace(err)
	}
	// We don't actually use the state for anything except authentication.
	defer releaser()

	switch machine := entity.(type) {
	case *state.Machine:
		// Only machines have machine tags.
		for _, job := range machine.Jobs() {
			if job == state.JobManageModel {
				return nil
			}
		}
	default:
		logger.Errorf("attempt to log in as a machine agent by %v", entity.Tag())
	}
	return errors.Trace(common.ErrPerm)
}

// ServeHTTP implements the http.Handler interface.
func (h *pubsubHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	handler := func(socket *websocket.Conn) {
		logger.Debugf("start of *pubsubHandler.ServeHTTP")
		defer socket.Close()

		if err := h.authenticate(req); err != nil {
			h.sendError(socket, req, err)
			return
		}

		// If we get to here, no more errors to report, so we report a nil
		// error.  This way the first line of the socket is always a json
		// formatted simple error.
		h.sendError(socket, req, nil)

		// Here we configure the ping/pong handling for the websocket so
		// the server can notice when the client goes away.
		// See the long note in logsink.go for the rationale.
		socket.SetReadDeadline(time.Now().Add(pongDelay))
		socket.SetPongHandler(func(string) error {
			socket.SetReadDeadline(time.Now().Add(pongDelay))
			return nil
		})
		ticker := time.NewTicker(pingPeriod)
		defer ticker.Stop()

		messageCh := h.receiveMessages(socket)
		for {
			select {
			case <-h.ctxt.stop():
				return
			case <-ticker.C:
				deadline := time.Now().Add(writeWait)
				if err := socket.WriteControl(websocket.PingMessage, []byte{}, deadline); err != nil {
					// This error is expected if the other end goes away. By
					// returning we close the socket through the defer call.
					logger.Debugf("failed to write ping: %s", err)
					return
				}
			case m := <-messageCh:
				logger.Tracef("topic: %q, data: %v", m.Topic, m.Data)
				_, err := h.hub.Publish(m.Topic, m.Data)
				if err != nil {
					logger.Errorf("publish failed: %v", err)
				}
			}
		}
	}
	websocketServer(w, req, handler)
}

func (h *pubsubHandler) receiveMessages(socket *websocket.Conn) <-chan params.PubSubMessage {
	messageCh := make(chan params.PubSubMessage)

	go func() {
		for {
			// The message needs to be new each time through the loop to ensure
			// the map is not reused.
			var m params.PubSubMessage
			// Receive() blocks until data arrives but will also be
			// unblocked when the API handler calls socket.Close as it
			// finishes.
			if err := socket.ReadJSON(&m); err != nil {
				logger.Errorf("pubsub receive error: %v", err)
				return
			}

			// Send the log message.
			select {
			case <-h.ctxt.stop():
				return
			case messageCh <- m:
			}
		}
	}()

	return messageCh
}

// sendError sends a JSON-encoded error response.
func (h *pubsubHandler) sendError(ws *websocket.Conn, req *http.Request, err error) {
	// There is no need to log the error for normal operators as there is nothing
	// they can action. This is for developers.
	if err != nil && featureflag.Enabled(feature.DeveloperMode) {
		logger.Errorf("returning error from %s %s: %s", req.Method, req.URL.Path, errors.Details(err))
	}
	if sendErr := sendInitialErrorV0(ws, err); sendErr != nil {
		logger.Errorf("closing websocket, %v", err)
		ws.Close()
		return
	}
}
