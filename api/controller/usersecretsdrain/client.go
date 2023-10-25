// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usersecretsdrain

import (
	"github.com/juju/juju/api/base"
	commonsecretbackends "github.com/juju/juju/api/common/secretbackends"
	commonsecretdrain "github.com/juju/juju/api/common/secretsdrain"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

type backendsClient = commonsecretbackends.Client

// Client is the api client for the UserSecretsDrain facade.
type Client struct {
	*commonsecretdrain.Client
	*backendsClient
}

// NewClient creates a secrets api client.
func NewClient(caller base.APICaller, options ...Option) *Client {
	facade := base.NewFacadeCaller(caller, "UserSecretsDrain", options...)
	return &Client{
		commonsecretdrain.NewClient(facade),
		commonsecretbackends.NewClient(facade),
	}
}
