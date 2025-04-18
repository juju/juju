// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"github.com/juju/errors"
	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/core/virtualhostname"
)

// SSHConnector is an interface that defines the methods required to
// connect to a remote SSH server.
type SSHConnector interface {
	Connect(destination virtualhostname.Info) (*gossh.Client, error)
}

// Logger is an interface that defines the methods required to log messages.
type Logger interface {
	Errorf(string, ...interface{})
	Debugf(string, ...interface{})
}

// Handlers provides a set of handlers for SSH sessions to machines.
type Handlers struct {
	connector   SSHConnector
	logger      Logger
	destination virtualhostname.Info
}

// NewHandlers creates a new set of machine handlers.
func NewHandlers(destination virtualhostname.Info, connector SSHConnector, logger Logger) (*Handlers, error) {
	if connector == nil {
		return nil, errors.NotValidf("connector is required")
	}
	if logger == nil {
		return nil, errors.NotValidf("logger is required")
	}
	if destination.Target() != virtualhostname.MachineTarget &&
		destination.Target() != virtualhostname.UnitTarget {
		return nil, errors.NotValidf("destination must be a machine or unit target")
	}
	return &Handlers{
		connector:   connector,
		logger:      logger,
		destination: destination,
	}, nil
}
