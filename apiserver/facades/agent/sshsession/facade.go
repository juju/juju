// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshsession

import (
	"context"

	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// Facade is the agent-facing SSH session facade. It allows a machine agent's
// sshsession worker to watch for and read one-shot SSH connection requests for
// its model, in order to establish reverse SSH tunnels back to the controller.
type Facade struct {
	// authorizer identifies the calling agent. The machine identity is always
	// taken from authentication, never from the caller's arguments, so an agent
	// can only observe and read connection requests targeting its own machine.
	authorizer                  facade.Authorizer
	service                     SSHConnRequestService
	controllerConfigService     ControllerConfigService
	controllerSSHHostKeyService ControllerSSHHostKeyService
	watcherRegistry             facade.WatcherRegistry
}

// newFacade returns a new SSH session facade.
func newFacade(
	authorizer facade.Authorizer,
	service SSHConnRequestService,
	controllerConfigService ControllerConfigService,
	controllerSSHHostKeyService ControllerSSHHostKeyService,
	watcherRegistry facade.WatcherRegistry,
) *Facade {
	return &Facade{
		authorizer:                  authorizer,
		service:                     service,
		controllerConfigService:     controllerConfigService,
		controllerSSHHostKeyService: controllerSSHHostKeyService,
		watcherRegistry:             watcherRegistry,
	}
}

// authMachineName returns the name of the authenticated machine agent. The
// identity comes solely from authentication, so a machine agent cannot request
// another machine's connection requests by supplying a different name.
func (f *Facade) authMachineName() (coremachine.Name, error) {
	machineTag, ok := f.authorizer.GetAuthTag().(names.MachineTag)
	if !ok {
		return "", apiservererrors.ErrPerm
	}
	return coremachine.Name(machineTag.Id()), nil
}

// WatchSSHConnRequest starts a watcher that emits the tunnel IDs of SSH
// connection requests targeting the authenticated machine. The machine is
// derived from authentication (not from the caller), and scoping happens in the
// domain service so an agent can only observe requests for its own machine.
func (f *Facade) WatchSSHConnRequest(ctx context.Context) (params.StringsWatchResult, error) {
	machineName, err := f.authMachineName()
	if err != nil {
		return params.StringsWatchResult{}, err
	}

	w, err := f.service.WatchSSHConnRequest(ctx, machineName)
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
	machineName, err := f.authMachineName()
	if err != nil {
		return params.SSHConnRequestResult{}, err
	}

	// The machine is scoped in the service/state query, so a machine can only
	// ever read its own request. This blocks the request up front rather than
	// fetching another machine's request and rejecting it afterwards, avoiding
	// leaking that request's credentials and reducing the attack surface.
	req, err := f.service.GetSSHConnRequest(ctx, machineName, arg.TunnelID)
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
	port, err := f.controllerConfigService.GetSSHServerPort(ctx)
	if err != nil {
		return params.SSHControllerSSHPortResult{}, errors.Errorf("getting controller SSH server port: %w", err)
	}
	return params.SSHControllerSSHPortResult{Port: port}, nil
}

// ControllerPublicKey returns the marshalled public host key of the controller
// SSH jump server. The machine agent uses it to pin the host key when
// reverse-dialling the controller. The public key derivation and caching live
// in the service, so the facade never handles private key material.
func (f *Facade) ControllerPublicKey(ctx context.Context) (params.SSHControllerPublicKeyResult, error) {
	publicKey, err := f.controllerSSHHostKeyService.SSHServerHostPublicKey(ctx)
	if err != nil {
		return params.SSHControllerPublicKeyResult{}, errors.Errorf("getting controller SSH host public key: %w", err)
	}
	return params.SSHControllerPublicKeyResult{
		PublicKey: publicKey,
	}, nil
}
