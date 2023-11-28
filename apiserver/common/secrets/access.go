// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/leadership"
	coresecrets "github.com/juju/juju/core/secrets"
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

// IsSameApplication returns true if the authenticated entity and the specified entity are in the same application.
func IsSameApplication(authTag names.Tag, tag names.Tag) bool {
	return AuthTagApp(authTag) == AuthTagApp(tag)
}

// CanManage checks that the authenticated caller can manage the secret, and returns a
// token to ensure leadership if that is required; ie if the request is for a secret
// owned by an application, the entity must be the unit leader.
func CanManage(
	api SecretsConsumer, leadershipChecker leadership.Checker,
	authTag names.Tag, uri *coresecrets.URI,
) (leadership.Token, error) {
	appName := AuthTagApp(authTag)
	appTag := names.NewApplicationTag(appName)

	switch authTag.(type) {
	case names.UnitTag:
		hasRole, err := api.SecretAccess(uri, authTag)
		if err != nil {
			// Typically not found error.
			return nil, errors.Trace(err)
		}
		if hasRole.Allowed(coresecrets.RoleManage) {
			// owner unit.
			return successfulToken{}, nil
		}
		hasRole, err = api.SecretAccess(uri, appTag)
		if err != nil {
			// Typically not found error.
			return nil, errors.Trace(err)
		}
		if hasRole.Allowed(coresecrets.RoleManage) {
			// leader unit can manage app owned secret.
			return LeadershipToken(authTag, leadershipChecker)
		}
	case names.ApplicationTag:
		// TODO(wallyworld) - remove auth tag kind check when podspec charms are gone.
		hasRole, err := api.SecretAccess(uri, appTag)
		if err != nil {
			// Typically not found error.
			return nil, errors.Trace(err)
		}
		if hasRole.Allowed(coresecrets.RoleManage) {
			return successfulToken{}, nil
		}
	case names.ModelTag:
		hasRole, err := api.SecretAccess(uri, authTag)
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

// CanRead returns true if the specified entity can read the secret.
func CanRead(api SecretsConsumer, authTag names.Tag, uri *coresecrets.URI, entity names.Tag) (bool, error) {
	// First try looking up unit access.
	hasRole, err := api.SecretAccess(uri, entity)
	if err != nil {
		// Typically not found error.
		return false, errors.Trace(err)
	}
	if hasRole.Allowed(coresecrets.RoleView) {
		return true, nil
	}

	// 1. all units can read secrets owned by application.
	// 2. units of podspec applications can do this as well.
	appName := AuthTagApp(authTag)
	hasRole, err = api.SecretAccess(uri, names.NewApplicationTag(appName))
	if err != nil {
		// Typically not found error.
		return false, errors.Trace(err)
	}
	return hasRole.Allowed(coresecrets.RoleView), nil
}

// OwnerToken returns a token used to determine if the specified entity
// is owned by the authenticated caller.
func OwnerToken(authTag names.Tag, ownerTag names.Tag, leadershipChecker leadership.Checker) (leadership.Token, error) {
	if !IsSameApplication(authTag, ownerTag) {
		return nil, apiservererrors.ErrPerm
	}
	// A unit can create a secret so long as the
	// secret owner is that unit's app.
	// TODO(wallyworld) - remove auth tag kind check when podspec charms are gone.
	if authTag.Kind() == names.ApplicationTagKind || authTag.Id() == ownerTag.Id() {
		return successfulToken{}, nil
	}
	return LeadershipToken(authTag, leadershipChecker)
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
