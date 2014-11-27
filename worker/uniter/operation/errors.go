// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"github.com/juju/errors"
)

var (
	ErrNoStateFile = errors.New("uniter state file does not exist")
	ErrSkipExecute = errors.New("operation already executed")
	ErrNeedsReboot = errors.New("reboot request issued")
	ErrHookFailed  = errors.New("hook failed")
)
