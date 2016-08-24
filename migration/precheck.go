// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"github.com/juju/errors"
)

// SourcePrecheckBackend defines the things required to
type SourcePrecheckBackend interface {
	NeedsCleanup() (bool, error)
}

// SourcePrecheck checks the state of the source controller to make
// sure that the preconditions for model migration are met.
func SourcePrecheck(backend SourcePrecheckBackend) error {
	cleanupNeeded, err := backend.NeedsCleanup()
	if err != nil {
		return errors.Annotate(err, "checking cleanups")
	}
	if cleanupNeeded {
		return errors.New("precheck failed: cleanup needed")
	}
	return nil
}
