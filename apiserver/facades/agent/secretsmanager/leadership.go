// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/leadership"
)

func ownerToken(authTag names.Tag, ownerTag names.Tag, leadershipChecker leadership.Checker) (leadership.Token, error) {
	if !isSameApplication(authTag, ownerTag) {
		return nil, apiservererrors.ErrPerm
	}
	// A unit can create a secret so long as the
	// secret owner is that unit's app.
	if authTag.Id() == ownerTag.Id() {
		return successfulToken{}, nil
	}
	return LeadershipToken(authTag, leadershipChecker)
}

// isSameApplication returns true if the authenticated entity and the specified entity are in the same application.
func isSameApplication(authTag names.Tag, tag names.Tag) bool {
	return authTagApp(authTag) == authTagApp(tag)
}

// isLeaderUnit returns true if the authenticated caller is the unit leader of its application.
func isLeaderUnit(authTag names.Tag, leadershipChecker leadership.Checker) (bool, error) {
	if authTag.Kind() != names.UnitTagKind {
		return false, nil
	}
	_, err := LeadershipToken(authTag, leadershipChecker)
	if err != nil && !leadership.IsNotLeaderError(err) {
		return false, errors.Trace(err)
	}
	return err == nil, nil
}

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

type successfulToken struct{}

// Check implements lease.Token.
func (t successfulToken) Check() error {
	return nil
}

// LeadershipToken returns a token used to determine if the authenticated
// caller is the unit leader of its application.
func LeadershipToken(authTag names.Tag, leadershipChecker leadership.Checker) (leadership.Token, error) {
	appName := authTagApp(authTag)
	token := leadershipChecker.LeadershipCheck(appName, authTag.Id())
	if err := token.Check(); err != nil {
		return nil, errors.Trace(err)
	}
	return token, nil
}
