// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsrevoker

import (
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// Client is the api client for the SecretRevoker facade.
type Client struct {
	facade base.FacadeCaller
}

// NewClient creates a secret revoker API client.
func NewClient(caller base.APICaller) *Client {
	return &Client{
		facade: base.NewFacadeCaller(caller, "SecretsRevoker"),
	}
}

// WatchIssuedTokenExpiry calls the SecretsRevoker facade to create a secret
// backends issued token expiry watcher. The watcher fires when a secret backend
// issued token is created, sending the RFC3339 encoded timestamp when it will
// expire.
func (c *Client) WatchIssuedTokenExpiry() (watcher.StringsWatcher, error) {
	var result params.StringsWatchResult
	err := c.facade.FacadeCall("WatchIssuedTokenExpiry", nil, &result)
	if err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, apiservererrors.RestoreError(result.Error)
	}
	w := apiwatcher.NewStringsWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

// RevokeIssuedTokens calls the SecretsRevoker facade to revoke all issued
// tokens up until the specified time.
func (c *Client) RevokeIssuedTokens(until time.Time) error {
	var result params.ErrorResult
	err := c.facade.FacadeCall("RevokeIssuedTokens", until, &result)
	if err != nil {
		return errors.Trace(err)
	}
	if result.Error != nil {
		return result.Error
	}
	return nil
}
