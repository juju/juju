// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"github.com/juju/names/v4"

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

func (s *SecretsManagerAPI) canManage(uri *coresecrets.URI, entity names.Tag) bool {
	// Manage access is granted on a per app basis;
	// Any unit from the allowed app can manage.
	authAppName := authTagApp(entity)
	if authAppName == "" {
		return false
	}
	app := names.NewApplicationTag(authAppName)
	return s.hasRole(uri, app, coresecrets.RoleManage)
}

func (s *SecretsManagerAPI) canRead(uri *coresecrets.URI, entity names.Tag) bool {
	if s.canManage(uri, entity) {
		return true
	}
	// First try looking up unit access.
	hasRole, _ := s.secretsConsumer.SecretAccess(uri, entity)
	if hasRole.Allowed(coresecrets.RoleView) {
		return true
	}
	// If no unit level access granted, try for the app.
	authAppName := authTagApp(entity)
	if authAppName == "" {
		return false
	}
	app := names.NewApplicationTag(authAppName)
	hasRole, _ = s.secretsConsumer.SecretAccess(uri, app)
	return hasRole.Allowed(coresecrets.RoleView)
}
