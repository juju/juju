// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raft

import (
	"fmt"

	"github.com/juju/errors"
)

// NotLeaderError creates a typed error for when a raft operation is applied,
// but the raft state shows that it's not the leader. The error will help
// redirect the consumer of the error to workout where they can try and find
// the leader.
type NotLeaderError struct {
	serverAddress string
	serverID      string
}

func (e *NotLeaderError) Error() string {
	return fmt.Sprintf("not currently the leader, try %q", e.serverID)
}

// ServerAddress returns the address of the potential current leader. It's not
// guaranteed to be the leader, as things may of changed when attempting the
// same request on the new leader.
func (e *NotLeaderError) ServerAddress() string {
	return e.serverAddress
}

// ServerID returns the server ID from the raft state. This should align with
// the controller machine ID of Juju.
func (e *NotLeaderError) ServerID() string {
	return e.serverID
}

// AsMap returns a map of the error. Useful when crossing the facade boundary
// and wanting information in the client.
func (e *NotLeaderError) AsMap() map[string]interface{} {
	return map[string]interface{}{
		"server-address": e.serverAddress,
		"server-id":      e.serverID,
	}
}

// NewNotLeaderError creates a new NotLeaderError with the server address and/or
// server ID of the current raft state leader.
func NewNotLeaderError(serverAddress, serverID string) error {
	return &NotLeaderError{
		serverAddress: serverAddress,
		serverID:      serverID,
	}
}

// IsNotLeaderError returns true if the error is the NotLeaderError.
func IsNotLeaderError(err error) bool {
	_, ok := errors.Cause(err).(*NotLeaderError)
	return ok
}
