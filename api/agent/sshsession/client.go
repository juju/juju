// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshsession

import (
	"context"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

// Client provides access to the SSHSession agent facade, used by the machine
// agent's sshsession worker to watch and read SSH connection requests.
type Client struct {
	facade base.FacadeCaller
}

// NewClient returns a new SSHSession facade client.
func NewClient(caller base.APICaller, options ...Option) *Client {
	return &Client{facade: base.NewFacadeCaller(caller, "SSHSession", options...)}
}

// WatchSSHConnRequest returns a strings watcher that emits the tunnel IDs of
// SSH connection requests in the model.
func (c *Client) WatchSSHConnRequest(ctx context.Context) (watcher.StringsWatcher, error) {
	var result params.StringsWatchResult
	if err := c.facade.FacadeCall(ctx, "WatchSSHConnRequest", nil, &result); err != nil {
		return nil, errors.Capture(err)
	}
	if result.Error != nil {
		return nil, result.Error
	}
	return apiwatcher.NewStringsWatcher(c.facade.RawAPICaller(), result), nil
}

// GetSSHConnRequest returns the SSH connection request identified by tunnelID.
func (c *Client) GetSSHConnRequest(ctx context.Context, tunnelID string) (params.SSHConnRequestResult, error) {
	var result params.SSHConnRequestResult
	arg := params.SSHConnRequestArg{TunnelID: tunnelID}
	if err := c.facade.FacadeCall(ctx, "GetSSHConnRequest", arg, &result); err != nil {
		return params.SSHConnRequestResult{}, errors.Capture(err)
	}
	if result.Error != nil {
		return params.SSHConnRequestResult{}, apiservererrors.RestoreError(result.Error)
	}
	return result, nil
}

// ControllerSSHPort returns the port the controller SSH jump server listens on.
func (c *Client) ControllerSSHPort(ctx context.Context) (int, error) {
	var result params.SSHControllerSSHPortResult
	if err := c.facade.FacadeCall(ctx, "ControllerSSHPort", nil, &result); err != nil {
		return 0, errors.Capture(err)
	}
	if result.Error != nil {
		return 0, apiservererrors.RestoreError(result.Error)
	}
	return result.Port, nil
}

// ControllerPublicKey returns the marshalled public host key of the controller
// SSH jump server, used to pin the host key when reverse-dialling.
func (c *Client) ControllerPublicKey(ctx context.Context) ([]byte, error) {
	var result params.SSHControllerPublicKeyResult
	if err := c.facade.FacadeCall(ctx, "ControllerPublicKey", nil, &result); err != nil {
		return nil, errors.Capture(err)
	}
	if result.Error != nil {
		return nil, apiservererrors.RestoreError(result.Error)
	}
	return result.PublicKey, nil
}
