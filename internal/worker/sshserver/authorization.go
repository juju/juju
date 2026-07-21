// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"context"

	"github.com/gliderlabs/ssh"
	"github.com/juju/errors"
	"github.com/lestrrat-go/jwx/v2/jwt"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/virtualhostname"
)

// AccessService checks local user access to an SSH target.
type AccessService interface {
	// HasSSHAccess checks if the given username has SSH access to the specified destination.
	HasSSHAccess(context.Context, string, virtualhostname.Info) (bool, error)
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
	if !ok {
		return false, errors.New("SSH authentication method is missing from connection context")
	}
	if publicKey {
		ok, err := a.access.HasSSHAccess(ctx, ctx.User(), destination)
		if err != nil {
			return false, errors.Annotate(err, "checking SSH access")
		}
		return ok, nil
	}

	token, _ := ctx.Value(userJWT{}).(jwt.Token)
	if token == nil {
		return false, errors.New("SSH JWT is missing from connection context")
	}

	claims, ok := token.PrivateClaims()["access"].(map[string]any)
	if !ok {
		return false, errors.New("invalid SSH JWT token, missing access claim")
	}
	access, _ := claims["model-"+destination.ModelUUID().String()].(string)
	return permission.Access(access).EqualOrGreaterModelAccessThan(permission.AdminAccess), nil
}
