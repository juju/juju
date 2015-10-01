// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_client

import (
	"github.com/juju/errors"
	"github.com/lxc/lxd"
)

// Client is a high-level wrapper around the LXD API client.
type Client struct {
	raw       rawClientWrapper
	namespace string
}

// Connect opens an API connection to LXD and returns a high-level
// Client wrapper around that connection.
func Connect(cfg Config) (*Client, error) {
	raw, err := newRawClient(cfg.Remote)
	if err != nil {
		return nil, errors.Trace(err)
	}

	conn := &Client{
		raw:       raw,
		namespace: cfg.Namespace,
	}
	return conn, nil
}

// TODO(ericsnow) Support passing auth info to newRawClient?

func newRawClient(remote string) (*lxd.Client, error) {
	// TODO(ericsnow) Yuck! This write the config file to the current
	// user's home directory...
	cfg, err := lxd.LoadConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}

	client, err := lxd.NewClient(cfg, remote)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return client, nil
}
