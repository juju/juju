// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

import (
	"github.com/juju/errors"
)

// Config contains the config values used for a connection to the LXD API.
type Config struct {
	// Remote identifies the remote server to which the client should
	// connect. For the default "remote" use Local.
	Remote Remote
}

// WithDefaults updates a copy of the config with default values
// where needed.
func (cfg Config) WithDefaults() (Config, error) {
	// We leave a blank namespace alone.
	// Also, note that cfg is a value receiver, so it is an implicit copy.

	var err error
	cfg.Remote, err = cfg.Remote.WithDefaults()
	if err != nil {
		return cfg, errors.Trace(err)
	}
	return cfg, nil
}

// Validate checks the client's fields for invalid values.
func (cfg Config) Validate() error {
	if err := cfg.Remote.Validate(); err != nil {
		return errors.Trace(err)
	}

	return nil
}
