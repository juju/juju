// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshsession

import (
	"context"

	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// Facade is the agent-facing SSH session facade. It allows a machine agent's
// sshsession worker to watch for and read one-shot SSH connection requests for
// its model, in order to establish reverse SSH tunnels back to the controller.
type Facade struct {
	service                     SSHConnRequestService
	controllerConfigService     ControllerConfigService
	controllerSSHHostKeyService ControllerSSHHostKeyService
	watcherRegistry             facade.WatcherRegistry
}

// newFacade returns a new SSH session facade.
func newFacade(
	service SSHConnRequestService,
	controllerConfigService ControllerConfigService,
	controllerSSHHostKeyService ControllerSSHHostKeyService,
	watcherRegistry facade.WatcherRegistry,
) *Facade {
	return &Facade{
		service:                     service,
		controllerConfigService:     controllerConfigService,
		controllerSSHHostKeyService: controllerSSHHostKeyService,
		watcherRegistry:             watcherRegistry,
	}
}

// WatchSSHConnRequest starts a watcher that emits the tunnel IDs of SSH
// connection requests in the model. The machine agent worker sshsession
// filters the emitted tunnel IDs to those targeting its own machine.
func (f *Facade) WatchSSHConnRequest(ctx context.Context) (params.StringsWatchResult, error) {
	w, err := f.service.WatchSSHConnRequest(ctx)
	if err != nil {
		return params.StringsWatchResult{}, errors.Errorf("watching SSH connection requests: %w", err)
	}

	id, initial, err := internal.EnsureRegisterWatcher(ctx, f.watcherRegistry, w)
	if err != nil {
		return params.StringsWatchResult{}, errors.Errorf("registering SSH connection request watcher: %w", err)
	}
	return params.StringsWatchResult{
		StringsWatcherId: id,
		Changes:          initial,
	}, nil
}

// GetSSHConnRequest returns the SSH connection request identified by the
// supplied tunnel ID.
func (f *Facade) GetSSHConnRequest(ctx context.Context, arg params.SSHConnRequestArg) (params.SSHConnRequestResult, error) {
	req, err := f.service.GetSSHConnRequest(ctx, arg.TunnelID)
	if err != nil {
		return params.SSHConnRequestResult{}, errors.Errorf("getting SSH connection request %q: %w", arg.TunnelID, err)
	}

	addresses := make([]string, len(req.ControllerAddresses))
	for i, addr := range req.ControllerAddresses {
		addresses[i] = addr.Value
	}

	return params.SSHConnRequestResult{
		MachineName:         req.MachineName,
		ControllerAddresses: addresses,
		Username:            req.SSHUsername,
		Password:            req.SSHPassword,
		UnitPort:            req.UnitPort,
		EphemeralPublicKey:  req.EphemeralPublicKey,
	}, nil
}

// ControllerSSHPort returns the port the controller SSH jump server listens on.
func (f *Facade) ControllerSSHPort(ctx context.Context) (params.SSHControllerSSHPortResult, error) {
	cfg, err := f.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return params.SSHControllerSSHPortResult{}, errors.Errorf("getting controller config: %w", err)
	}
	return params.SSHControllerSSHPortResult{Port: cfg.SSHServerPort()}, nil
}

// ControllerPublicKey returns the marshalled public host key of the controller
// SSH jump server. The machine agent uses it to pin the host key when
// reverse-dialling the controller.
func (f *Facade) ControllerPublicKey(ctx context.Context) (params.SSHControllerPublicKeyResult, error) {
	privateHostKey, err := f.controllerSSHHostKeyService.SSHServerHostKey(ctx)
	if err != nil {
		return params.SSHControllerPublicKeyResult{}, errors.Errorf("getting controller SSH host key: %w", err)
	}

	signer, err := gossh.ParsePrivateKey([]byte(privateHostKey))
	if err != nil {
		return params.SSHControllerPublicKeyResult{}, errors.Errorf("parsing controller SSH host key: %w", err)
	}
	return params.SSHControllerPublicKeyResult{
		PublicKey: signer.PublicKey().Marshal(),
	}, nil
}
