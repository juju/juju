// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftleaseservice

import (
	"net/http"
	"time"

	gorillaws "github.com/gorilla/websocket"
	"github.com/juju/clock"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/websocket"
)

// Handler is an http.Handler suitable for serving lease consuming connections.
type Handler struct {
	operations chan<- operation
	abort      <-chan struct{}
	clock      clock.Clock
	logger     Logger
}

// NewHandler returns a new Handler that sends operations to the
// given operations channel, and stops accepting operations after
// the abort channel is closed.
func NewHandler(
	operations chan<- operation,
	abort <-chan struct{},
	clock clock.Clock,
	logger Logger,
) *Handler {
	return &Handler{
		operations: operations,
		abort:      abort,
		clock:      clock,
		logger:     logger,
	}
}

// ServeHTTP is part of the http.Handler interface.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	handler := func(socket *websocket.Conn) {
		h.logger.Debugf("start of *raftleaseservice.ServeHTTP")
		defer socket.Close()

		// If we get to here, no more errors to report, so we report a nil
		// error.  This way the first line of the socket is always a json
		// formatted simple error.
		h.sendError(socket, nil)

		// Ensure we set the read deadline on the sockets for the Ping/Pong
		// messages. That way we can workout if a client has actually gone away
		// or not.
		h.ensureReadDeadline(socket)

		ticker := time.NewTicker(websocket.PingPeriod)
		defer ticker.Stop()

		operations, closed := h.receiveOperations(socket)
		for {
			select {
			case <-h.abort:
				return
			case <-closed:
				// We received an error, there isn't much we can do here, other
				// than death.
				return
			case <-ticker.C:
				deadline := time.Now().Add(websocket.WriteWait)
				if err := socket.WriteControl(gorillaws.PingMessage, []byte{}, deadline); err != nil {
					// This error is expected if the other end goes away. By
					// returning we close the socket through the defer call.
					h.logger.Debugf("failed to write ping: %s", err)
					return
				}
			case op := <-operations:
				select {
				case <-h.abort:
					return
				case h.operations <- op:
				}
			}
		}
	}
	websocket.Serve(w, r, handler)
}

func (h *Handler) receiveOperations(socket *websocket.Conn) (<-chan operation, <-chan struct{}) {
	var (
		operations = make(chan operation)
		done       = make(chan struct{})
	)

	go func() {
		for {
			// The message needs to be new each time through the loop to ensure
			// the map is not reused.
			var m params.LeaseOperation
			// Receive() blocks until data arrives but will also be
			// unblocked when the API handler calls socket.Close as it
			// finishes.
			if err := socket.ReadJSON(&m); err != nil {
				if closed := h.handleSocketError(err); closed {
					close(done)
				}
				return
			}

			// Wrap the command in an operation. The operation has a callback so
			// that we can send a result to the socket.
			op := operation{
				Commands: []string{m.Command},
				Callback: func(err error) {
					// As some time may have passed, we want to ensure that we
					// haven't aborted in between the callback being called.
					select {
					case <-h.abort:
					default:
					}

					var pErr *params.Error
					if err != nil {
						pErr = &params.Error{
							Message: err.Error(),
							Code:    params.CodeBadRequest,
						}
					}

					result := params.LeaseOperationResult{
						UUID:  m.UUID,
						Error: pErr,
					}

					if err := socket.WriteJSON(&result); err != nil {
						if closed := h.handleSocketError(err); closed {
							close(done)
						}
					}
				},
			}

			// Send the log message.
			select {
			case <-h.abort:
				return
			case operations <- op:
			}
		}
	}()

	return operations, done
}

// Here we configure the ping/pong handling for the websocket so
// the server can notice when the client goes away.
// See the long note in logsink.go for the rationale.
func (h *Handler) ensureReadDeadline(socket *websocket.Conn) {
	pongDelay := h.clock.Now().Add(websocket.PongDelay)
	if err := socket.SetReadDeadline(pongDelay); err != nil {
		h.logger.Errorf("unable to set read deadline to %s", pongDelay)
	}
	socket.SetPongHandler(func(string) error {
		pongDelay := h.clock.Now().Add(websocket.PongDelay)
		if err := socket.SetReadDeadline(pongDelay); err != nil {
			h.logger.Errorf("unable to set pong read deadline to %s", pongDelay)
		}
		return nil
	})
}

func (h *Handler) handleSocketError(err error) bool {
	// Since we don't give a list of expected error codes,
	// any CloseError type is considered unexpected.
	if gorillaws.IsUnexpectedCloseError(err) {
		h.logger.Tracef("websocket closed")
		return true
	} else {
		h.logger.Errorf("raftleaseservice receive error: %v", err)
	}
	return false
}

// sendError sends a JSON-encoded error response.
func (h *Handler) sendError(ws *websocket.Conn, err error) {
	if sendErr := ws.SendInitialErrorV0(err); sendErr != nil {
		h.logger.Errorf("closing websocket, %v", err)
		ws.Close()
		return
	}
}
