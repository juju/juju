// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/leadership"
	coresecrets "github.com/juju/juju/core/secrets"
	secretservice "github.com/juju/juju/domain/secret/service"
)

// AuthTagApp returns the application name of the authenticated entity.
func AuthTagApp(authTag names.Tag) string {
	switch authTag.Kind() {
	case names.ApplicationTagKind:
		return authTag.Id()
	case names.UnitTagKind:
		authAppName, _ := names.UnitApplication(authTag.Id())
		return authAppName
	}
	return ""
}

// CanManage checks that the authenticated caller can manage the secret, and returns a
// token to ensure leadership if that is required; ie if the request is for a secret
// owned by an application, the entity must be the unit leader.
func CanManage(
	ctx context.Context,
	api SecretConsumer, leadershipChecker leadership.Checker,
	authTag names.Tag, uri *coresecrets.URI,
) (leadership.Token, error) {
	appName := AuthTagApp(authTag)

	switch authTag.(type) {
	case names.UnitTag:
		unitName := authTag.Id()
		hasRole, err := api.GetSecretAccess(ctx, uri, secretservice.SecretAccessor{
			Kind: secretservice.UnitAccessor, ID: unitName})
		if err != nil {
			// Typically not found error.
			return nil, errors.Trace(err)
		}
		if hasRole.Allowed(coresecrets.RoleManage) {
			// owner unit.
			return successfulToken{}, nil
		}
		hasRole, err = api.GetSecretAccess(ctx, uri, secretservice.SecretAccessor{
			Kind: secretservice.ApplicationAccessor, ID: appName})
		if err != nil {
			// Typically not found error.
			return nil, errors.Trace(err)
		}
		if hasRole.Allowed(coresecrets.RoleManage) {
			// leader unit can manage app owned secret.
			return LeadershipToken(authTag, leadershipChecker)
		}
	case names.ModelTag:
		modelUUID := authTag.Id()
		hasRole, err := api.GetSecretAccess(ctx, uri, secretservice.SecretAccessor{
			Kind: secretservice.ModelAccessor, ID: modelUUID})
		if err != nil {
			// Typically not found error.
			return nil, errors.Trace(err)
		}
		if hasRole.Allowed(coresecrets.RoleManage) {
			return successfulToken{}, nil
		}
	}
	return nil, apiservererrors.ErrPerm
}

type successfulToken struct{}

// Check implements lease.Token.
func (t successfulToken) Check() error {
	return nil
}

// LeadershipToken returns a token used to determine if the authenticated
// caller is the unit leader of its application.
func LeadershipToken(authTag names.Tag, leadershipChecker leadership.Checker) (leadership.Token, error) {
	appName := AuthTagApp(authTag)
	token := leadershipChecker.LeadershipCheck(appName, authTag.Id())
	if err := token.Check(); err != nil {
		return nil, errors.Trace(err)
	}
	return token, nil
}

// IsLeaderUnit returns true if the authenticated caller is the unit leader of its application.
func IsLeaderUnit(authTag names.Tag, leadershipChecker leadership.Checker) (bool, error) {
	if authTag.Kind() != names.UnitTagKind {
		return false, nil
	}
	_, err := LeadershipToken(authTag, leadershipChecker)
	if err != nil && !leadership.IsNotLeaderError(err) {
		return false, errors.Trace(err)
	}
	return err == nil, nil
}
