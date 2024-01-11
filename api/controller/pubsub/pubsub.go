// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pubsub

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/rpc/params"
)

// MessageWriter is the interface that allows sending pub/sub messges to the
// server.
type MessageWriter interface {
	// ForwardMessage forwards the given message to the server.
	ForwardMessage(*params.PubSubMessage) error
	Close() error
}

// API provides access to the pubsub API.
type API struct {
	connector base.StreamConnector
}

// NewAPI creates a new client-side pubsub API.
func NewAPI(connector base.StreamConnector) *API {
	return &API{connector: connector}
}

// OpenMessageWriter returns a new message writer interface value which must
// be closed when finished with.
func (api *API) OpenMessageWriter(ctx context.Context) (MessageWriter, error) {
	conn, err := api.connector.ConnectStream(ctx, "/pubsub", nil)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot connect to /pubsub")
	}
	messageWriter := &writer{conn}
	go messageWriter.readLoop()
	return messageWriter, nil
}

type writer struct {
	conn base.Stream
}

// readLoop is necessary for the client to process websocket control messages.
// Close() is safe to call concurrently.
func (w *writer) readLoop() {
	for {
		if _, _, err := w.conn.NextReader(); err != nil {
			w.conn.Close()
			break
		}
	}
}

func (w *writer) ForwardMessage(m *params.PubSubMessage) error {
	// Note: due to the fire-and-forget nature of the
	// pubsub API, it is possible that when the
	// connection dies, any messages that were "in-flight"
	// will not be received on the server side.
	if err := w.conn.WriteJSON(m); err != nil {
		return errors.Annotatef(err, "cannot send pubsub message")
	}
	return nil
}

func (w *writer) Close() error {
	return w.conn.Close()
}
