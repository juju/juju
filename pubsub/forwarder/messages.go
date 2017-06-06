// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package forwarder

const (
	// ConnectTopic is published when a connection is established between
	// the forwarding worker and another API server.
	ConnectedTopic = "worker.pubsub.remote.connect"

	// DisconnectedTopic is published when a connection is broken between
	// the forwarding worker and another API server.
	DisconnectedTopic = "worker.pubsub.remote.disconnect"
)

// OriginTarget represents the data for the connect and disconnect
// topics.
type OriginTarget struct {
	// Origin represents this API server.
	Origin string `yaml:"origin"`
	// Target represents the other API server that this one is forwarding
	// messages to.
	Target string `yaml:"target"`
}
