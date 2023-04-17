// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsdrain

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

func (s *SecretsDrainAPI) hasRole(uri *coresecrets.URI, entity names.Tag, role coresecrets.SecretRole) bool {
	hasRole, err := s.secretsConsumer.SecretAccess(uri, entity)
	return err == nil && hasRole.Allowed(role)
}

// canManage checks that the authenticated caller can manage the secret, and returns a
// token to ensure leadership if that is required; ie if the request is for a secret
// owned by an application, the entity must be the unit leader.
func (s *SecretsDrainAPI) canManage(uri *coresecrets.URI) (leadership.Token, error) {

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

type successfulToken struct{}

// Check implements lease.Token.
func (t successfulToken) Check() error {
	return nil
}

// leadershipToken returns a token used to determine if the authenticated
// caller is the unit leader of its application.
func (s *SecretsDrainAPI) leadershipToken() (leadership.Token, error) {
	appName := authTagApp(s.authTag)
	token := s.leadershipChecker.LeadershipCheck(appName, s.authTag.Id())
	if err := token.Check(); err != nil {
		return nil, errors.Trace(err)
	}
	return token, nil
}
