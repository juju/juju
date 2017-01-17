// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

import (
	"github.com/juju/errors"
	"github.com/lxc/lxd"
	"github.com/lxc/lxd/shared"
)

type rawConfigClient interface {
	SetServerConfig(key string, value string) (*lxd.Response, error)

	WaitForSuccess(waitURL string) error
	ServerStatus() (*shared.ServerState, error)
}

type configClient struct {
	raw rawConfigClient
}

// SetConfig sets the given value in the server's config.
func (c configClient) SetServerConfig(key, value string) error {
	resp, err := c.raw.SetServerConfig(key, value)
	if err != nil {
		return errors.Trace(err)
	}

	if resp.Operation != "" {
		if err := c.raw.WaitForSuccess(resp.Operation); err != nil {
			// TODO(ericsnow) Handle different failures (from the async
			// operation) differently?
			return errors.Trace(err)
		}
	}

	return nil
}

func (c configClient) ServerStatus() (*shared.ServerState, error) {
	return c.raw.ServerStatus()
}
