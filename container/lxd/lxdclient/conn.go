// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdclient

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

func newRawClient(remote Remote) (*lxd.Client, error) {
	cfg, err := lxd.LoadConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}

	client, err := lxd.NewClient(cfg, remote.ID())
	if err != nil {
		return nil, errors.Trace(err)
	}

	return client, nil
}
