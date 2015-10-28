// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdclient

import (
	"github.com/juju/errors"
	"github.com/lxc/lxd"
	"github.com/lxc/lxd/shared"
)

// Client is a high-level wrapper around the LXD API client.
type Client struct {
	*clientServerMethods

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
		clientServerMethods: &clientServerMethods{raw},

		raw:       raw,
		namespace: cfg.Namespace,
		remote:    remote,
		isLocal:   cfg.Remote.isLocal(),
	}
	return conn, nil
}

func newRawClient(remote, configDir string) (*lxd.Client, error) {
	logger.Debugf("loading LXD client config from %q", configDir)

	// This will go away once LoadConfig takes a dirname argument.
	updateLXDVars(configDir)

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

func prepareRemote(cfg Config, newCert Cert) error {
	client, err := Connect(cfg)
	if err != nil {
		return errors.Trace(err)
	}

	if err := client.SetConfig("core.https_address", "[::]"); err != nil {
		return errors.Trace(err)
	}

	name, _ := shared.SplitExt(cfg.Filename) // TODO(ericsnow) Is this right?
	name = "juju-" + name
	if err := client.AddCert(newCert, name); err != nil {
		return errors.Trace(err)
	}

	return nil
}
