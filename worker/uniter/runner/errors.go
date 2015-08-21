// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner

import (
	"fmt"

	"github.com/juju/errors"
)

var ErrActionNotAvailable = errors.New("action no longer available")

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

func NewBadActionError(actionName, problem string) error {
	return &badActionError{actionName, problem}
}
