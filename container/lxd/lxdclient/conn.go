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
	remote    string
}

// Connect opens an API connection to LXD and returns a high-level
// Client wrapper around that connection.
func Connect(cfg Config) (*Client, error) {
	if err := cfg.Apply(); err != nil {
		return nil, errors.Trace(err)
	}
	remote := cfg.Remote.ID()

	raw, err := newRawClient(remote)
	if err != nil {
		return nil, errors.Trace(err)
	}

	conn := &Client{
		raw:       raw,
		namespace: cfg.Namespace,
		remote:    remote,
	}
	return conn, nil
}

func newRawClient(remote string) (*lxd.Client, error) {
	logger.Debugf("loading LXD client config from %q", lxd.ConfigDir)

	cfg, err := lxd.LoadConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}

	logger.Debugf("using LXD remote %q", remote)
	client, err := lxd.NewClient(cfg, remote)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return client, nil
}
