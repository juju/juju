// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

import (
	"github.com/juju/clock"
)

const (
	// ServiceName is the name of the service that Juju's mongod instance
	// will be named.
	ServiceName = "juju-db"

	// ReplicaSetName is the name of the replica set that juju uses for its
	// controllers.
	ReplicaSetName = "juju"
)

// ConfigArgs holds the attributes of a service configuration for mongo.
type ConfigArgs struct {
	Clock clock.Clock

	DataDir    string
	DBDir      string
	ReplicaSet string

	// connection params
	BindIP      string
	BindToAllIP bool
	Port        int
	OplogSizeMB int

	// auth
	AuthKeyFile    string
	PEMKeyFile     string
	PEMKeyPassword string

	// network params
	IPv6             bool
	TLSOnNormalPorts bool
	TLSMode          string

	// Logging. Syslog cannot be true with LogPath set to a non-empty string.
	// SlowMS is the threshold time in milliseconds that Mongo will consider an
	// operation to be slow, causing it to be written to the log.
	Syslog  bool
	LogPath string
	SlowMS  int

	// misc
	Quiet bool
}
