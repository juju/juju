// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pubsub

import (
	"context"
	"net/http"
	"time"

	gorillaws "github.com/gorilla/websocket"
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/websocket"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/featureflag"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/rpc/params"
)

// Hub defines the publish method that the handler uses to publish
// messages on the centralhub of the apiserver.
type Hub interface {
	Publish(string, interface{}) (func(), error)
}

type pubsubHandler struct {
	abort  <-chan struct{}
	hub    Hub
	logger corelogger.Logger
}

// NewPubSubHandler returns a new http.Handler that handles pubsub
// messages.
func NewPubSubHandler(abort <-chan struct{}, hub Hub) http.Handler {
	return &pubsubHandler{
		abort:  abort,
		hub:    hub,
		logger: internallogger.GetLogger("juju.apiserver.pubsub"),
	}
}

// ServeHTTP implements the http.Handler interface.
func (h *pubsubHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	handler := func(socket *websocket.Conn) {
		h.logger.Debugf(req.Context(), "start of *pubsubHandler.ServeHTTP")
		defer socket.Close()

		// If we get to here, no more errors to report, so we report a nil
		// error.  This way the first line of the socket is always a json
		// formatted simple error.
		h.sendError(socket, req, nil)

		// Here we configure the ping/pong handling for the websocket so
		// the server can notice when the client goes away.
		// See the long note in logsink.go for the rationale.
		_ = socket.SetReadDeadline(time.Now().Add(websocket.PongDelay))
		socket.SetPongHandler(func(string) error {
			_ = socket.SetReadDeadline(time.Now().Add(websocket.PongDelay))
			return nil
		})
		ticker := time.NewTicker(websocket.PingPeriod)
		defer ticker.Stop()

		messageCh := h.receiveMessages(socket)
		for {
			select {
			case <-h.abort:
				return
			case <-ticker.C:
				deadline := time.Now().Add(websocket.WriteWait)
				if err := socket.WriteControl(gorillaws.PingMessage, []byte{}, deadline); err != nil {
					// This error is expected if the other end goes away. By
					// returning we close the socket through the defer call.
					h.logger.Debugf(req.Context(), "failed to write ping: %s", err)
					return
				}
			case m := <-messageCh:
				h.logger.Tracef(req.Context(), "topic: %q, data: %v", m.Topic, m.Data)
				_, err := h.hub.Publish(m.Topic, m.Data)
				if err != nil {
					h.logger.Errorf(req.Context(), "publish failed: %v", err)
				}
			}
		}
	}
	websocket.Serve(w, req, handler)
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
				// Since we don't give a list of expected error codes,
				// any CloseError type is considered unexpected.
				if gorillaws.IsUnexpectedCloseError(err) {
					h.logger.Tracef(context.TODO(), "websocket closed")
				} else {
					h.logger.Errorf(context.TODO(), "pubsub receive error: %v", err)
				}
				return
			}

			// Send the log message.
			select {
			case <-h.abort:
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
	if err != nil && featureflag.Enabled(featureflag.DeveloperMode) {
		h.logger.Errorf(req.Context(), "returning error from %s %s: %s", req.Method, req.URL.Path, errors.Details(err))
	}
	if sendErr := ws.SendInitialErrorV0(err); sendErr != nil {
		h.logger.Errorf(req.Context(), "closing websocket, %v", err)
		ws.Close()
		return
	}
}
