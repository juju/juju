// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import "errors"

var (
	// ErrNotPrepared should be returned by providers when
	// an operation is attempted on an unprepared environment.
	ErrNotPrepared = errors.New("environment is not prepared")
	ErrDestroyed   = errors.New("environment has been destroyed")
)
