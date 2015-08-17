// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/errors"
)

var ErrRequeueAndReboot = errors.New("reboot now")
var ErrReboot = errors.New("reboot after hook")
var ErrNoProcess = errors.New("no process to kill")

type missingHookError struct {
	hookName string
}

func (e *missingHookError) Error() string {
	return e.hookName + " does not exist"
}

func IsMissingHookError(err error) bool {
	_, ok := err.(*missingHookError)
	return ok
}

func NewMissingHookError(hookName string) error {
	return &missingHookError{hookName}
}
