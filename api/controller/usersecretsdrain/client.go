// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usersecretsdrain

import (
	"github.com/juju/juju/api/base"
	commonsecretbackends "github.com/juju/juju/api/common/secretbackends"
	commonsecretdrain "github.com/juju/juju/api/common/secretsdrain"
)

type backendsClient = commonsecretbackends.Client

// Client is the api client for the UserSecretsDrain facade.
type Client struct {
	*commonsecretdrain.Client
	*backendsClient
}

// NewClient creates a secrets api client.
func NewClient(caller base.APICaller) *Client {
	facade := base.NewFacadeCaller(caller, "UserSecretsDrain")
	return &Client{
		commonsecretdrain.NewClient(facade),
		commonsecretbackends.NewClient(facade),
	}
}
