// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"github.com/gliderlabs/ssh"
	"github.com/juju/names/v5"
	"github.com/lestrrat-go/jwx/v2/jwt"

	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/virtualhostname"
)

type authorizer struct {
	facadeClient FacadeClient
	logger       Logger
}

// authorize checks whether the authenticated user has access to the
// specified destination.
//
// If the user was authenticated via public key, we can assume the user
// is a local user and check the controller's state to see if they have
// access. If the user was authenticated via JWT, we assume they are an
// external user and check the JWT claims instead.
func (a *authorizer) authorize(ctx ssh.Context, destination virtualhostname.Info) bool {
	authenticatedViaPublicKey, ok := ctx.Value(authenticatedViaPublicKey{}).(bool)
	if !ok {
		a.logger.Errorf("failed to get authenticatedViaPublicKey from context")
		return false
	}

	if authenticatedViaPublicKey {
		ok, err := a.facadeClient.CheckSSHAccess(ctx.User(), destination)
		if err != nil {
			a.logger.Errorf("failed to check SSH access: %v", err)
			return false
		}
		return ok
	} else {
		token, _ := ctx.Value(userJWT{}).(jwt.Token)
		if token == nil {
			a.logger.Errorf("failed to get jwt token from context")
			return false
		}
		return a.checkSSHAccessViaJWT(token, destination)
	}
}

func (a *authorizer) checkSSHAccessViaJWT(token jwt.Token, destination virtualhostname.Info) bool {
	modelTag := names.NewModelTag(destination.ModelUUID())
	accessClaims, ok := token.PrivateClaims()["access"].(map[string]interface{})
	if !ok || len(accessClaims) == 0 {
		return false
	}
	modelAccess, _ := accessClaims[modelTag.String()].(string)
	return modelAccess == string(permission.AdminAccess)
}
