// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"github.com/juju/errors"

	"github.com/juju/juju/process"
)

type Client struct {
}

func NewClient() (*Client, error) {
	// TODO(ericsnow) finish
	return &Client{}, errors.Errorf("not finished")
}

func (c *Client) List() ([]string, error) {
	// TODO(ericsnow) finish
	return nil, errors.Errorf("not finished")
}

func (c *Client) Get(ids ...string) ([]*process.Info, error) {
	// TODO(ericsnow) finish
	return nil, errors.Errorf("not finished")
}

func (c *Client) Set(procs ...*process.Info) error {
	// TODO(ericsnow) finish
	return errors.Errorf("not finished")
}
