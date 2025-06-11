// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"time"

	"github.com/juju/juju/api"
	"github.com/juju/juju/core/migration"
)

// NewLoginProvider returns a LoginProvider for the target controller
// using the given TargetInfo. If TargetInfo contains a session token,
// it creates a SessionTokenLoginProvider for authenticating with JIMM controllers.
// If no session token is present, it returns nil, which is treated as legacy
// authentication i.e. username/password or macaroons.
func NewLoginProvider(targetInfo migration.TargetInfo) api.LoginProvider {
	if targetInfo.Token != "" {
		return api.NewSessionTokenLoginProvider(targetInfo.Token, nil, nil)
	}
	return nil
}

// ControllerDialOpts returns dial parameters suitable for connecting
// from the source controller to the target controller during model
// migrations.
// Except for the inclusion of RetryDelay the options mirror what is used
// by the APICaller for logins.
func ControllerDialOpts(loginProvider api.LoginProvider) api.DialOpts {
	return api.DialOpts{
		LoginProvider:       loginProvider,
		DialTimeout:         3 * time.Second,
		DialAddressInterval: 200 * time.Millisecond,
		Timeout:             time.Minute,
		RetryDelay:          100 * time.Millisecond,
	}
}
