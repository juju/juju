// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

import (
	"github.com/juju/errors"
	"github.com/lxc/lxd"
)

// Client is a high-level wrapper around the LXD API client.
type Client struct {
	*serverConfigClient
	*certClient
	*profileClient

	raw       rawClientWrapper
	namespace string
	remote    string
	isLocal   bool
}

// Connect opens an API connection to LXD and returns a high-level
// Client wrapper around that connection.
func Connect(cfg Config) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	// TODO(ericsnow) Call cfg.Write here if necessary?
	remote := cfg.Remote.ID()

	raw, err := newRawClient(remote, cfg.Dirname)
	if err != nil {
		return nil, errors.Trace(err)
	}

	conn := &Client{
		serverConfigClient: &serverConfigClient{raw},
		certClient:         &certClient{raw},
		profileClient:      &profileClient{raw},

		raw:       raw,
		namespace: cfg.Namespace,
		remote:    remote,
		isLocal:   cfg.Remote.isLocal(),
	}
	return conn, nil
}

var lxdNewClient = lxd.NewClient
var lxdLoadConfig = lxd.LoadConfig

func newRawClient(remote, configDir string) (*lxd.Client, error) {
	logger.Debugf("loading LXD client config from %q", configDir)

	// This will go away once LoadConfig takes a dirname argument.
	origDirname := updateLXDVars(configDir)
	defer updateLXDVars(origDirname)

	cfg, err := lxdLoadConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}

	logger.Debugf("using LXD remote %q", remote)
	client, err := lxdNewClient(cfg, remote)
	if err != nil {
		if remote == remoteIDForLocal {
			return nil, errors.Annotate(err, "can't connect to the local LXD server")
		}
		return nil, errors.Trace(err)
	}
	return client, nil
}
