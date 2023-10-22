// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsbackendmanager

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// Client is the api client for the SecretBackendsManager facade.
type Client struct {
	facade base.FacadeCaller
}

// NewClient creates a secret backends manager api client.
func NewClient(caller base.APICaller) *Client {
	return &Client{
		facade: base.NewFacadeCaller(caller, "SecretBackendsManager"),
	}
}

// WatchTokenRotationChanges returns a watcher that triggers on secret
// backend rotation changes.
func (c *Client) WatchTokenRotationChanges() (watcher.SecretBackendRotateWatcher, error) {
	var result params.SecretBackendRotateWatchResult
	err := c.facade.FacadeCall(context.TODO(), "WatchSecretBackendsRotateChanges", nil, &result)
	if err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, apiservererrors.RestoreError(result.Error)
	}
	w := apiwatcher.NewSecretBackendRotateWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

// RotateBackendTokens rotates the tokens for the specified secret backends.
func (c *Client) RotateBackendTokens(backendIDs ...string) error {
	var results params.ErrorResults
	err := c.facade.FacadeCall(context.TODO(), "RotateBackendTokens", params.RotateSecretBackendArgs{
		BackendIDs: backendIDs,
	}, &results)
	if err != nil {
		return errors.Trace(err)
	}
	return results.Combine()
}
