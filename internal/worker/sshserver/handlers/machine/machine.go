// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package machine provides SSH handlers for machine and machine-unit targets.
package machine

import (
	"context"

	"github.com/juju/errors"
	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/virtualhostname"
)

// SSHConnector establishes SSH clients to machine targets.
type SSHConnector interface {
	// Connect establishes an SSH client to the given target.
	// If the target is not a machine or machine unit, an error is returned.
	Connect(context.Context, virtualhostname.Info) (*gossh.Client, error)
}

// Handlers provides SSH channel handlers for a machine target.
type Handlers struct {
	connector   SSHConnector
	logger      logger.Logger
	destination virtualhostname.Info
}

// NewHandlers returns handlers for a machine or machine-unit target.
func NewHandlers(destination virtualhostname.Info, connector SSHConnector, logger logger.Logger) (*Handlers, error) {
	if connector == nil {
		return nil, errors.New("connector is required")
	}
	if logger == nil {
		return nil, errors.New("logger is required")
	}
	if destination.Target() != virtualhostname.MachineTarget &&
		destination.Target() != virtualhostname.UnitTarget {
		return nil, errors.New("destination must be a machine or unit target")
	}
	return &Handlers{
		connector:   connector,
		logger:      logger,
		destination: destination,
	}, nil
}
