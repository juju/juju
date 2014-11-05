// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"fmt"
)

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

type badActionError struct {
	actionName string
	problem    string
}

func (e *badActionError) Error() string {
	return fmt.Sprintf("cannot run %q action: %s", e.actionName, e.problem)
}

func IsBadActionError(err error) bool {
	_, ok := err.(*badActionError)
	return ok
}
