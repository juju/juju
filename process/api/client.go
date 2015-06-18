// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"github.com/juju/errors"

	"github.com/juju/juju/process"
)

// Client provides methods for interacting with the Juju API relative
// to workload processes.
type Client struct {
}

// NewClient builds a new workload process API client.
func NewClient() (*Client, error) {
	// TODO(ericsnow) finish
	return &Client{}, errors.Errorf("not finished")
}

// List gets the list of defined workload processes from Juju
// via the API.
func (c *Client) List() ([]string, error) {
	// TODO(ericsnow) finish
	return nil, errors.Errorf("not finished")
}

// Get gets the info for the specified workload processes via the API.
func (c *Client) Get(ids ...string) ([]*process.Info, error) {
	// TODO(ericsnow) finish
	return nil, errors.Errorf("not finished")
}

// Set pushes the provided process info up to Juju state via the API.
func (c *Client) Set(procs ...*process.Info) error {
	// TODO(ericsnow) finish
	return errors.Errorf("not finished")
}
