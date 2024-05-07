// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dualwritehack

import (
	"github.com/juju/errors"

	"github.com/juju/juju/internal/servicefactory"
	"github.com/juju/juju/state"
)

// Logger defines the methods used by the dual write worker/
type Logger interface {
	Infof(string, ...interface{})
}

// Config holds all necessary attributes to start a dual write worker.
type Config struct {
	StatePool      *state.StatePool
	ServiceFactory servicefactory.ServiceFactory
	Logger         Logger
}

// Validate will err unless basic requirements for a valid
// config are met.
func (c *Config) Validate() error {
	if c.ServiceFactory == nil {
		return errors.New("missing ServiceFactory")
	}
	if c.Logger == nil {
		return errors.New("missing Logger")
	}
	return nil
}
