// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"fmt"

	"github.com/juju/errors"
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

// DeployConflictError is returned by the deploy operation when the charm cannot be
// deployed.
type DeployConflictError struct {
	charmURL string
}

func (err *DeployConflictError) Error() string {
	return fmt.Sprintf("cannot deploy charm %s", err.charmURL)
}

// NewDeployConflictError returns an error indicating that the charm with
// the supplied URL failed to deploy.
func NewDeployConflictError(charmURL string) error {
	return &DeployConflictError{charmURL}
}
