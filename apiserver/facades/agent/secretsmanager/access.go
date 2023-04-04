// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/leadership"
	coresecrets "github.com/juju/juju/core/secrets"
)

// authTagApp returns the application name of the authenticated entity.
func authTagApp(authTag names.Tag) string {
	switch authTag.Kind() {
	case names.ApplicationTagKind:
		return authTag.Id()
	case names.UnitTagKind:
		authAppName, _ := names.UnitApplication(authTag.Id())
		return authAppName
	}
	return ""
}

func (s *SecretsManagerAPI) hasRole(uri *coresecrets.URI, entity names.Tag, role coresecrets.SecretRole) bool {
	hasRole, err := s.secretsConsumer.SecretAccess(uri, entity)
	return err == nil && hasRole.Allowed(role)
}

// canManage checks that the authenticated caller can manage the secret, and returns a
// token to ensure leadership if that is required; ie if the request is for a secret
// owned by an application, the entity must be the unit leader.
func (s *SecretsManagerAPI) canManage(uri *coresecrets.URI) (leadership.Token, error) {

	appName := authTagApp(s.authTag)
	appTag := names.NewApplicationTag(appName)

	switch s.authTag.(type) {
	case names.UnitTag:
		if s.hasRole(uri, s.authTag, coresecrets.RoleManage) {
			// owner unit.
			return successfulToken{}, nil
		}
		if s.hasRole(uri, appTag, coresecrets.RoleManage) {
			// leader unit can manage app owned secret.
			return s.leadershipToken()
		}
	case names.ApplicationTag:
		// TODO(wallyworld) - remove auth tag kind check when podspec charms are gone.
		if s.hasRole(uri, appTag, coresecrets.RoleManage) {
			return successfulToken{}, nil
		}
	}
	return nil, apiservererrors.ErrPerm
}

// canRead returns true if the specified entity can read the secret.
func (s *SecretsManagerAPI) canRead(uri *coresecrets.URI, entity names.Tag) bool {
	// First try looking up unit access.
	hasRole, _ := s.secretsConsumer.SecretAccess(uri, entity)
	if hasRole.Allowed(coresecrets.RoleView) {
		return true
	}

	// 1. all units can read secrets owned by application.
	// 2. units of podspec applications can do this as well.
	appName := authTagApp(s.authTag)
	hasRole, _ = s.secretsConsumer.SecretAccess(uri, names.NewApplicationTag(appName))
	return hasRole.Allowed(coresecrets.RoleView)
}

func (s *SecretsManagerAPI) isSameApplication(tag names.Tag) bool {
	return authTagApp(s.authTag) == authTagApp(tag)
}

// ownerToken returns a token used to determine if the specified entity
// is owned by the authenticated caller.
func (s *SecretsManagerAPI) ownerToken(ownerTag names.Tag) (leadership.Token, error) {
	if !s.isSameApplication(ownerTag) {
		return nil, apiservererrors.ErrPerm
	}
	// A unit can create a secret so long as the
	// secret owner is that unit's app.
	// TODO(wallyworld) - remove auth tag kind check when podspec charms are gone.
	if s.authTag.Kind() == names.ApplicationTagKind || s.authTag.Id() == ownerTag.Id() {
		return successfulToken{}, nil
	}
	return s.leadershipToken()
}

type successfulToken struct{}

// Check implements lease.Token.
func (t successfulToken) Check() error {
	return nil
}

// leadershipToken returns a token used to determine if the authenticated
// caller is the unit leader of its application.
func (s *SecretsManagerAPI) leadershipToken() (leadership.Token, error) {
	appName := authTagApp(s.authTag)
	token := s.leadershipChecker.LeadershipCheck(appName, s.authTag.Id())
	if err := token.Check(); err != nil {
		return nil, errors.Trace(err)
	}
	return token, nil
}
