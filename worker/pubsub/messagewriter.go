// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pubsub

import (
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/api"
	pubsubapi "github.com/juju/juju/api/controller/pubsub"
	"github.com/juju/juju/rpc/params"
)

// MessageWriter defines the two methods called for message forwarding.
type MessageWriter interface {
	// ForwardMessage forwards the given message to the server.
	ForwardMessage(*params.PubSubMessage) error
	Close()
}

var dialOpts = api.DialOpts{
	DialAddressInterval: 20 * time.Millisecond,
	// If for some reason we are getting rate limited, there is a standard
	// five second delay before we get the login response. Ideally we need
	// to wait long enough for this response to get back to us.
	// Ideally the apiserver wouldn't be rate limiting connections from other
	// API servers, see bug #1733256.
	Timeout:    10 * time.Second,
	RetryDelay: 1 * time.Second,
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
