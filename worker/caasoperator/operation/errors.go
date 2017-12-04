// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE store for details.

package operation

import "github.com/juju/errors"

var (
	ErrSkipExecute = errors.New("operation already executed")
	ErrHookFailed  = errors.New("hook failed")
)
