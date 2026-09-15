// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"context"

	"github.com/juju/errors"
	ssh "github.com/tailscale/gliderssh"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/virtualhostname"
)

// AccessService checks local user access to an SSH target.
type AccessService interface {
	// HasSSHAccessToModel checks if the given username has SSH access to the specified destination.
	HasSSHAccessToModel(context.Context, string, virtualhostname.Info) (bool, error)
}

type authorizer struct {
	access AccessService
	logger logger.Logger
}

// Authorize checks if the SSH connection context is authorized to access the target destination.
// By this point, we expect the authenticator to have set the authentication method and
// any relevant claims in the context.
func (a authorizer) Authorize(ctx ssh.Context, destination virtualhostname.Info) (bool, error) {
	publicKey, ok := ctx.Value(authenticatedViaPublicKey{}).(bool)
	if !ok || !publicKey {
		return false, errors.New("SSH connection is not authenticated via public key")
	}
	ok, err := a.access.HasSSHAccessToModel(ctx, ctx.User(), destination)
	if err != nil {
		return false, errors.Annotate(err, "checking SSH access")
	}
	return ok, nil
}
