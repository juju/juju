// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usersecrets

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// Client is the api client for the UserSecretsManager facade.
type Client struct {
	facade base.FacadeCaller
}

// NewClient creates a secret backends manager api client.
func NewClient(caller base.APICaller) *Client {
	return &Client{
		facade: base.NewFacadeCaller(caller, "UserSecretsManager"),
	}
}

// WatchRevisionsToPrune returns a watcher that triggers on secret
// obsolete revision changes.
func (c *Client) WatchRevisionsToPrune() (watcher.StringsWatcher, error) {
	var result params.StringsWatchResult
	err := c.facade.FacadeCall(context.TODO(), "WatchRevisionsToPrune", nil, &result)
	if err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, params.TranslateWellKnownError(result.Error)
	}
	w := apiwatcher.NewStringsWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

// DeleteRevisions deletes the specified revisions of the secret.
func (c *Client) DeleteRevisions(uri *secrets.URI, revisions ...int) error {
	if uri == nil {
		return errors.Errorf("uri cannot be nil")
	}
	if len(revisions) == 0 {
		return errors.Errorf("at least one revision must be specified")
	}
	arg := params.DeleteSecretArg{
		URI:       uri.String(),
		Revisions: revisions,
	}

	var results params.ErrorResults
	err := c.facade.FacadeCall(context.TODO(), "DeleteRevisions", params.DeleteSecretArgs{Args: []params.DeleteSecretArg{arg}}, &results)
	if err != nil {
		return errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return errors.Errorf("unexpected number of results: %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return params.TranslateWellKnownError(result.Error)
	}
	return nil
}
