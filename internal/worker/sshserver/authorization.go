// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"context"

	"github.com/gliderlabs/ssh"
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
func (a authorizer) Authorize(ctx ssh.Context, destination virtualhostname.Info) bool {
	publicKey, ok := ctx.Value(authenticatedViaPublicKey{}).(bool)
	if !ok {
		a.logger.Errorf(ctx, "SSH authentication method is missing from connection context")
		return false
	}
	if publicKey {
		ok, err := a.access.HasSSHAccess(ctx, ctx.User(), destination)
		if err != nil {
			a.logger.Errorf(ctx, "checking SSH access: %v", err)
			return false
		}
		return ok
	}

	token, _ := ctx.Value(userJWT{}).(jwt.Token)
	if token == nil {
		a.logger.Warningf(ctx, "SSH JWT is missing from connection context")
		return false
	}

	claims, ok := token.PrivateClaims()["access"].(map[string]any)
	if !ok {
		a.logger.Warningf(ctx, "Invalid SSH JWT token, missing access claim")
		return false
	}
	access, _ := claims["model-"+destination.ModelUUID().String()].(string)
	return permission.Access(access).EqualOrGreaterModelAccessThan(permission.AdminAccess)
}
