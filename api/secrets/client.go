// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/secrets"
)

// Client is the api client for the Secrets facade.
type Client struct {
	facade base.FacadeCaller
}

// NewClient creates a secrets api client.
func NewClient(caller base.APICaller) *Client {
	return &Client{
		facade: base.NewFacadeCaller(caller, "Secrets"),
	}
}

// Create creates a new secret.
func (c *Client) Create(name string, value secrets.SecretValue) (string, error) {
	return "", errors.NotImplementedf("Create Secret")
}

// GetValue returns the value of a secret.
func (c *Client) GetValue(name string) (secrets.SecretValue, error) {
	return nil, errors.NotImplementedf("Get Secret")
}
