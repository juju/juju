// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"fmt"

	"github.com/juju/errors"
	corecharm "gopkg.in/juju/charm.v5"
)

var (
	ErrNoStateFile            = errors.New("uniter state file does not exist")
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

// DeployConflictCharmURL returns the charm URL used to create the supplied
// deploy conflict error, and a bool indicating success.
func DeployConflictCharmURL(err error) (*corecharm.URL, bool) {
	if e, ok := err.(*deployConflictError); ok {
		return e.charmURL, true
	}
	return nil, false
}
