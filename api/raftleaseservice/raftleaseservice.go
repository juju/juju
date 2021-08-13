// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package raftleaseservice implements the API for streaming raft lease messages
// between api servers.
package raftleaseservice

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

// MessageWriter is the interface that allows sending messages to the
// server.
type MessageWriter interface {
	// Send sends the given message to the server.
	Send(*params.LeaseOperation) error
	Close() error
}

// API provides access to the pubsub API.
type API struct {
	connector base.ControllerStreamConnector
}

// NewAPI creates a new client-side pubsub API.
func NewAPI(connector base.ControllerStreamConnector) *API {
	return &API{connector: connector}
}

// OpenMessageWriter returns a new message writer interface value which must
// be closed when finished with.
func (api *API) OpenMessageWriter() (MessageWriter, error) {
	conn, err := api.connector.ConnectControllerStream("/raft/lease", nil, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot connect to /raft/lease")
	}
	messageWriter := &writer{
		conn: conn,
		done: make(chan struct{}),
	}
	go messageWriter.readLoop()
	return messageWriter, nil
}

type writer struct {
	conn base.Stream
	done chan struct{}
}

// readLoop is necessary for the client to process websocket control messages.
// Close() is safe to call concurrently.
func (w *writer) readLoop() {
	for {
		if _, _, err := w.conn.NextReader(); err != nil {
			close(w.done)
			w.conn.Close()
			break
		}
	}
}

func (w *writer) Send(m *params.LeaseOperation) error {
	if err := w.conn.WriteJSON(m); err != nil {
		return errors.Annotatef(err, "cannot send lease operation message")
	}

	// TODO (stickupkid): We should limit the number of attempts, otherwise it's
	// just a hungry hippo.
	var readOp params.LeaseOperationResult
	for {
		select {
		case <-w.done:
			return nil
		default:
		}

		// We're expecting a result back, as these can be out of order, we're
		// going to consume all the operations until we find the one we actually
		// care about.
		if err := w.conn.ReadJSON(&readOp); err != nil {
			return errors.Trace(err)
		}
		if readOp.UUID == m.UUID {
			break
		}
	}
	return readOp.Error
}

func (w *writer) Close() error {
	close(w.done)
	return w.conn.Close()
}
