// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package forwarder

import "github.com/juju/juju/internal/pubsub/common"

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
type OriginTarget common.OriginTarget
