// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pubsub

import (
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/api"
	pubsubapi "github.com/juju/juju/api/pubsub"
	"github.com/juju/juju/apiserver/params"
)

// MessageWriter defines the two methods called for message forwarding.
type MessageWriter interface {
	// ForwardMessage forwards the given message to the server.
	ForwardMessage(*params.PubSubMessage) error
	Close()
}

var dialOpts = api.DialOpts{
	DialAddressInterval: 20 * time.Millisecond,
	Timeout:             50 * time.Millisecond,
	RetryDelay:          50 * time.Millisecond,
}

// NewMessageWriter will connect to the remote defined by the info,
// and return a MessageWriter.
func NewMessageWriter(info *api.Info) (MessageWriter, error) {
	conn, err := api.Open(info, dialOpts)
	if err != nil {
		return nil, errors.Trace(err)
	}
	a := pubsubapi.NewAPI(conn)
	writer, err := a.OpenMessageWriter()
	if err != nil {
		conn.Close()
		return nil, errors.Trace(err)
	}
	return &remoteConnection{connection: conn, MessageWriter: writer}, nil
}

// remoteConnection represents an api connection to another
// API server for the purpose of forwarding pubsub messages.
type remoteConnection struct {
	connection api.Connection

	pubsubapi.MessageWriter
}

func (r *remoteConnection) Close() {
	r.MessageWriter.Close()
	r.connection.Close()
}
