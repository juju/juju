// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package auth

import "github.com/juju/juju/internal/errors"

const (
	// ErrStopAuthentication is a special error that can be returned by an
	// [Authenticator] to indicate to the caller that no further authentication
	// workflows MUST be performed. The Authenticator has reached an error which
	// it believes cannot be resolved.
	ErrStopAuthentication errors.ConstError = "stop authentication"
)
