// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"fmt"

	"github.com/juju/errors"
	corecharm "gopkg.in/juju/charm.v6"
)

var (
	// ErrNoSavedState is returned by StateOps if there is no saved state.  This is
	// usually seen when a unit starts for the first time.
	ErrNoSavedState           = errors.New("saved uniter state does not exist")
	ErrSkipExecute            = errors.New("operation already executed")
	ErrNeedsReboot            = errors.New("reboot request issued")
	ErrHookFailed             = errors.New("hook failed")
	ErrCannotAcceptLeadership = errors.New("cannot accept leadership")
)

type deployConflictError struct {
	charmURL *corecharm.URL
}

func (err *deployConflictError) Error() string {
	return fmt.Sprintf("cannot deploy charm %s", err.charmURL)
}

// NewDeployConflictError returns an error indicating that the charm with
// the supplied URL failed to deploy.
func NewDeployConflictError(charmURL *corecharm.URL) error {
	return &deployConflictError{charmURL}
}

// IsDeployConflictError returns true if the error is a
// deploy conflict error.
func IsDeployConflictError(err error) bool {
	_, ok := err.(*deployConflictError)
	return ok
}
