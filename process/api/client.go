// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"github.com/juju/errors"

	"github.com/juju/juju/process"
)

// InternalClient provides methods for interacting with Juju's internal
// RPC API, relative to workload processes.
type InternalClient struct {
}

// NewInternalClient builds a new workload process API client.
func NewInternalClient() (*InternalClient, error) {
	client := &InternalClient{}

	// TODO(ericsnow) finish
	return client, errors.Errorf("not finished")
}

// List gets the list of defined workload processes from Juju
// via the API.
func (c *InternalClient) List() ([]string, error) {
	// TODO(ericsnow) finish
	return nil, errors.Errorf("not finished")
}

// Get gets the info for the specified workload processes via the API.
func (c *InternalClient) Get(ids ...string) ([]*process.Info, error) {
	// TODO(ericsnow) finish
	return nil, errors.Errorf("not finished")
}

// Set pushes the provided process info up to Juju state via the API.
func (c *InternalClient) Set(procs ...*process.Info) error {
	// TODO(ericsnow) finish
	return errors.Errorf("not finished")
}
