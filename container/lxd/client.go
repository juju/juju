// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"github.com/juju/errors"
	"github.com/lxc/lxd/client"
)

// Client extends the upstream LXD container server.
type Client struct {
	ImageServer
	NetworkServer
}

// NewClient builds and returns a Client for high-level interaction with the
// input LXD container server.
func NewClient(svr lxd.ContainerServer) *Client {
	info, _, _ := svr.GetServer()
	apiExt := info.APIExtensions
	return &Client{
		ImageServer:   ImageServer{svr},
		NetworkServer: NewNetworkServer(svr, apiExt),
	}
}

// UpdateServerConfig updates the server configuration with the input values.
func (c *Client) UpdateServerConfig(cfg map[string]string) error {
	svr, eTag, err := c.ImageServer.GetServer()
	if err != nil {
		return errors.Trace(err)
	}
	if svr.Config == nil {
		svr.Config = make(map[string]interface{})
	}
	for k, v := range cfg {
		svr.Config[k] = v
	}
	return errors.Trace(c.ImageServer.UpdateServer(svr.Writable(), eTag))
}

// UpdateContainerConfig updates the configuration for the container with the
// input name, using the input values.
func (c *Client) UpdateContainerConfig(name string, cfg map[string]string) error {
	container, eTag, err := c.ImageServer.GetContainer(name)
	if err != nil {
		return errors.Trace(err)
	}
	if container.Config == nil {
		container.Config = make(map[string]string)
	}
	for k, v := range cfg {
		container.Config[k] = v
	}

	resp, err := c.ImageServer.UpdateContainer(name, container.Writable(), eTag)
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(resp.Wait())
}

func isLXDNotFound(err error) bool {
	return err.Error() == "not found"
}
