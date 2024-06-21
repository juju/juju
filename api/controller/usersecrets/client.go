// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usersecrets

import (
	"context"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

// Client is the api client for the UserSecretsManager facade.
type Client struct {
	facade base.FacadeCaller
}

// NewClient creates a secret backends manager api client.
func NewClient(caller base.APICaller, options ...Option) *Client {
	return &Client{
		facade: base.NewFacadeCaller(caller, "UserSecretsManager", options...),
	}
}

// WatchRevisionsToPrune returns a watcher that triggers on secret
// obsolete revision changes.
func (c *Client) WatchRevisionsToPrune() (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult
	err := c.facade.FacadeCall(context.TODO(), "WatchRevisionsToPrune", nil, &result)
	if err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, params.TranslateWellKnownError(result.Error)
	}
	w := apiwatcher.NewNotifyWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

// DeleteObsoleteUserSecrets deletes any obsolete user secret revisions.
func (c *Client) DeleteObsoleteUserSecretRevisions(ctx context.Context) error {
	return c.facade.FacadeCall(ctx, "DeleteObsoleteUserSecretRevisions", nil, nil)
}
