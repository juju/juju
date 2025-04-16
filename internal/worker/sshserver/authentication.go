// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"context"
	"encoding/base64"

	"github.com/gliderlabs/ssh"
	"github.com/juju/errors"
	"github.com/lestrrat-go/jwx/v2/jwt"
	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/core/virtualhostname"
	"github.com/juju/juju/rpc/params"
)

type authenticatedViaPublicKey struct{}

type userJWT struct{}

// JWTParser defines an interface to parse JWTs.
type JWTParser interface {
	Parse(ctx context.Context, token string) (jwt.Token, error)
}

// authenticator is used to authenticate users' ssh connections.
//
// Its verification methods should only be used at the initial
// jump server. For authentication at the terminating server,
// use the TerminatingServerPublicKeyAuthentication method.
type authenticator struct {
	logger       Logger
	jwtParser    JWTParser
	facadeClient FacadeClient
}

// newAuthenticator create an Authenticator for SSH connections.
func newAuthenticator(jwtParser JWTParser, logger Logger, facadeClient FacadeClient) (*authenticator, error) {
	if logger == nil {
		return nil, errors.Errorf("logger can't be nil.")
	}
	if jwtParser == nil {
		return nil, errors.Errorf("JWTParser can't be nil.")
	}
	if facadeClient == nil {
		return nil, errors.Errorf("FacadeClient can't be nil.")
	}

	return &authenticator{
		logger:       logger,
		jwtParser:    jwtParser,
		facadeClient: facadeClient,
	}, nil
}

// TODO(JUJU-7777): implement public key authentication in the jump server in addition to the terminating server.
//
// publicKeyAuthentication validates the provided user's public key against all authorized keys and search for a match.
// If it's found it returns true, in case of errors or no-match returns false.
func (auth authenticator) publicKeyAuthentication(ctx ssh.Context, key ssh.PublicKey) bool {
	ctx.SetValue(authenticatedViaPublicKey{}, true)
	return true
}

// passwordAuthentication verifies the password for the user is a valid JWT, normally issued
// by JIMM for federated auth. It sets the user's JWT in the context if the password is valid.
func (auth authenticator) passwordAuthentication(ctx ssh.Context, password string) bool {
	ctx.SetValue(authenticatedViaPublicKey{}, false)
	// If the authenticating user is jimm, we can assume the password
	// is a JWT. Otherwise we can't assume anything.
	if ctx.User() == "jimm" {
		token, err := auth.jwtParser.Parse(ctx, password)
		if err != nil {
			auth.logger.Errorf("failed to parse jwt token: %v", err)
			return false
		}
		ctx.SetValue(userJWT{}, token)
		return true
	}
	return false
}

// newTerminatingServerAuthenticator creates an authenticator that can be used
// within the terminating SSH server.
//
// This method uses the base authenticator to retrieve public keys from either
// the model the user is targeting or from the user's JWT token.
func (auth authenticator) newTerminatingServerAuthenticator(ctx ssh.Context, targetInfo virtualhostname.Info) (terminatingServerAuthenticator, error) {
	authenticatedViaPublicKey, ok := ctx.Value(authenticatedViaPublicKey{}).(bool)
	if !ok {
		return terminatingServerAuthenticator{}, errors.New("failed to get authenticatedViaPublicKey from context")
	}

	var tsa terminatingServerAuthenticator

	if authenticatedViaPublicKey {
		// if the user is authenticated via public key, we need to verify the key
		// against the model's authorized keys.
		sshPkiAuthArgs := params.ListAuthorizedKeysArgs{
			ModelUUID: targetInfo.ModelUUID(),
		}

		var err error
		tsa.keysToVerify, err = auth.facadeClient.ListPublicKeysForModel(sshPkiAuthArgs)
		if err != nil {
			return tsa, errors.Annotate(err, "failed to fetch public keys for model")
		}

	} else {
		// if the user is not authenticated via public key, we need to verify the
		// key against the public keys in the jwt claims.
		jwt, _ := ctx.Value(userJWT{}).(jwt.Token)
		if jwt == nil {
			return tsa, errors.New("failed to get jwt token from context")
		}

		jwtPublicKeyB64, ok := jwt.PrivateClaims()["ssh_public_key"].(string)
		if !ok {
			return tsa, errors.New("failed to get public key from token")
		}

		decodedJwtPublicKey, err := base64.StdEncoding.DecodeString(jwtPublicKeyB64)
		if err != nil {
			return tsa, errors.Annotate(err, "failed to decode public key from token")
		}

		publicKey, err := gossh.ParsePublicKey(decodedJwtPublicKey)
		if err != nil {
			return tsa, errors.Annotate(err, "failed to parse public key from token")
		}

		tsa.keysToVerify = []gossh.PublicKey{publicKey}
	}

	return tsa, nil
}

// terminatingServerAuthenticator SSH connections at the terminating server.
// This struct is derived from the base authenticator.
type terminatingServerAuthenticator struct {
	keysToVerify []gossh.PublicKey
}

// PublicKeyAuthentication verifies the public key provided by the user matches
// one of the keys in the context. It verifies that the user authenticated at
// the jump server is the same as the one at the terminating server.
func (tsa terminatingServerAuthenticator) PublicKeyAuthentication(ctx ssh.Context, publicKey ssh.PublicKey) bool {
	for _, key := range tsa.keysToVerify {
		if ssh.KeysEqual(key, publicKey) {
			return true
		}
	}
	return false
}
