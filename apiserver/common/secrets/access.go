// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/leadership"
)

type successfulToken struct{}

// Check implements lease.Token.
func (t successfulToken) Check() error {
	return nil
}

// LeadershipToken returns a token used to determine if the authenticated
// caller is the unit leader of its application.
func LeadershipToken(authTag names.Tag, leadershipChecker leadership.Checker) (leadership.Token, error) {
	switch authTag.(type) {
	case names.UnitTag:
		appName, _ := names.UnitApplication(authTag.Id())
		token := leadershipChecker.LeadershipCheck(appName, authTag.Id())
		return token, nil
	case names.ModelTag:
		return successfulToken{}, nil
	}
	// Should never happen.
	return nil, errors.Errorf("unexpected auth tag type %T", authTag)
}
