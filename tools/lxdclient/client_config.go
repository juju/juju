// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

import (
	"github.com/juju/errors"
	lxdclient "github.com/lxc/lxd/client"
	"github.com/lxc/lxd/shared/api"
)

type rawConfigClient interface {
	GetConnectionInfo() (info *lxdclient.ConnectionInfo, err error)
	GetContainer(name string) (container *api.Container, ETag string, err error)
	UpdateContainer(name string, container api.ContainerPut, ETag string) (op lxdclient.Operation, err error)
	GetServer() (server *api.Server, ETag string, err error)
	UpdateServer(server api.ServerPut, ETag string) (err error)
}

type configClient struct {
	raw rawConfigClient
}

// SetServerConfig sets the given value in the server's config.
func (c configClient) SetServerConfig(key, value string) error {
	server, _, err := c.raw.GetServer()
	if err != nil {
		return errors.Trace(err)
	}
	server.Config[key] = value
	return errors.Trace(c.raw.UpdateServer(server.Writable(), ""))
}

// SetContainerConfig sets the given config value for the specified
// container.
func (c configClient) SetContainerConfig(name, key, value string) error {
	container, _, err := c.raw.GetContainer(name)
	if err != nil {
		return errors.Trace(err)
	}
	container.Config[key] = value
	resp, err := c.raw.UpdateContainer(name, container.Writable(), "")
	if err != nil {
		return errors.Trace(err)
	}
	if err := resp.Wait(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// ServerStatus reports the state of the server.
func (c configClient) ServerStatus() (*api.Server, error) {
	server, _, err := c.raw.GetServer()
	return server, errors.Trace(err)
}

// ServerAddresses reports the addresses that the server is listening on.
func (c configClient) ServerAddresses() ([]string, error) {
	info, err := c.raw.GetConnectionInfo()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return info.Addresses, nil
}
