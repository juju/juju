// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"github.com/juju/errors"
)

// PrecheckBackend is implemented by *state.State but defined as an interface
// for easier testing.
type PrecheckBackend interface {
	NeedsCleanup() (bool, error)
}

// Precheck checks the database state to make sure that the preconditions
// for model migration are met.
func Precheck(backend PrecheckBackend) error {
	cleanupNeeded, err := backend.NeedsCleanup()
	if err != nil {
		return errors.Annotate(err, "precheck cleanups")
	}
	if cleanupNeeded {
		return errors.New("precheck failed: cleanup needed")
	}
	return nil
}
