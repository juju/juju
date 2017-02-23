// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"io"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/pubsub"
	"golang.org/x/net/websocket"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

// Hub defines the publish method that the handler uses to publish
// messages on the centralhub of the apiserver.
type Hub interface {
	Publish(pubsub.Topic, interface{}) (<-chan struct{}, error)
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
	server := websocket.Server{
		Handler: func(socket *websocket.Conn) {
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

			messageCh := h.receiveMessages(socket)
			for {
				select {
				case <-h.ctxt.stop():
					return
				case m := <-messageCh:
					logger.Tracef("topic: %q, data: %v", m.Topic, m.Data)
					_, err := h.hub.Publish(pubsub.Topic(m.Topic), m.Data)
					if err != nil {
						logger.Errorf("publish failed: %v", err)
					}
				}
			}
		},
	}
	server.ServeHTTP(w, req)
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
			if err := websocket.JSON.Receive(socket, &m); err != nil {
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
func (h *pubsubHandler) sendError(w io.Writer, req *http.Request, err error) {
	if err != nil {
		logger.Errorf("returning error from %s %s: %s", req.Method, req.URL.Path, errors.Details(err))
	}
	sendJSON(w, &params.ErrorResult{
		Error: common.ServerError(err),
	})
}
