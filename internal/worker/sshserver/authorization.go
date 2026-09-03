// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"context"

	"github.com/juju/errors"
	"github.com/lestrrat-go/jwx/v3/jwt"
	ssh "github.com/tailscale/gliderssh"
	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/virtualhostname"
)

// AccessService checks local user access to an SSH target and verifies that
// the public key used for auth is associated with that target's model.
type AccessService interface {
	// HasSSHAccessToModel checks if the given username has SSH access to the specified destination.
	HasSSHAccessToModel(context.Context, string, virtualhostname.Info) (bool, error)

	// PublicKeyInModel reports whether the given public key is registered for
	// the user on the model identified by the destination.
	PublicKeyInModel(context.Context, string, gossh.PublicKey, virtualhostname.Info) (bool, error)
}

type authorizer struct {
	access AccessService
	logger logger.Logger
}

// Authorize checks if the SSH connection context is authorized to access the target destination.
// By this point, we expect the authenticator to have set the authentication method and
// any relevant claims in the context.
func (a authorizer) Authorize(ctx ssh.Context, destination virtualhostname.Info) (bool, error) {
	// If the context does not contain the user's key then they did
	// not authenticate with a public key (e.g. JWT or reverse tunnel).
	publicKey, ok := ctx.Value(authenticatedPublicKey{}).(ssh.PublicKey)
	if ok {
		hasAccess, err := a.access.HasSSHAccessToModel(ctx, ctx.User(), destination)
		if err != nil {
			return false, errors.Annotate(err, "checking SSH access")
		}
		if !hasAccess {
			return false, nil
		}

		// The key used during authentication is controller scoped, but keys are
		// added per model and kept this way for compatibility with Juju 3. Now
		// that the model is known, verify the key the user is associated with it.
		inModel, err := a.access.PublicKeyInModel(ctx, ctx.User(), publicKey, destination)
		if err != nil {
			return false, errors.Annotate(err, "checking SSH key for model")
		}
		if !inModel {
			return false, errors.Errorf(
				"public key used to authenticate is not associated with model %q, add the key to the model to access it",
				destination.ModelUUID())
		}
		return true, nil
	}

	token, _ := ctx.Value(userJWT{}).(jwt.Token)
	if token == nil {
		return false, errors.New("SSH JWT is missing from connection context")
	}

	var rawClaims any
	if err := token.Get("access", &rawClaims); err != nil {
		return false, errors.New("invalid SSH JWT token, missing access claim")
	}
	claims, ok := rawClaims.(map[string]any)
	if !ok {
		return false, errors.New("invalid SSH JWT token, invalid access claim")
	}
	access, _ := claims["model-"+destination.ModelUUID().String()].(string)
	return permission.Access(access).EqualOrGreaterModelAccessThan(permission.AdminAccess), nil
}
