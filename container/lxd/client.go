// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"github.com/juju/errors"
	"github.com/lxc/lxd/client"
	"github.com/lxc/lxd/shared"
)

// Client extends the upstream LXD container server.
type Client struct {
	lxd.ContainerServer

	networkAPISupport bool
	clusterAPISupport bool

	localBridgeName string
}

// NewClient builds and returns a Client for high-level interaction with the
// input LXD container server.
func NewClient(svr lxd.ContainerServer) *Client {
	info, _, _ := svr.GetServer()
	apiExt := info.APIExtensions
	return &Client{
		ContainerServer:   svr,
		networkAPISupport: shared.StringInSlice("network", apiExt),
		clusterAPISupport: shared.StringInSlice("clustering", apiExt),
	}
}

// UpdateServerConfig updates the server configuration with the input values.
func (c *Client) UpdateServerConfig(cfg map[string]string) error {
	svr, eTag, err := c.GetServer()
	if err != nil {
		return errors.Trace(err)
	}
	if svr.Config == nil {
		svr.Config = make(map[string]interface{})
	}
	for k, v := range cfg {
		svr.Config[k] = v
	}
	return errors.Trace(c.UpdateServer(svr.Writable(), eTag))
}

// UpdateContainerConfig updates the configuration for the container with the
// input name, using the input values.
func (c *Client) UpdateContainerConfig(name string, cfg map[string]string) error {
	container, eTag, err := c.GetContainer(name)
	if err != nil {
		return errors.Trace(err)
	}
	if container.Config == nil {
		container.Config = make(map[string]string)
	}
	for k, v := range cfg {
		container.Config[k] = v
	}

	resp, err := c.UpdateContainer(name, container.Writable(), eTag)
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(resp.Wait())
}

func isLXDNotFound(err error) bool {
	return err.Error() == "not found"
}
