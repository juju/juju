// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/lifeflag"
)

// ErrRemoved may be returned by some worker started from Manifolds to
// indicate that the model under management no longer exists.
var ErrRemoved = errors.New("model removed")

// lifeFilter is used with the lifeflag manifolds -- which do not depend
// on runFlag -- to return an error that will be trapped by IsFatal.
func lifeFilter(err error) error {
	cause := errors.Cause(err)
	if cause == lifeflag.ErrNotFound {
		return ErrRemoved
	}
	return err
}

// IsFatal will probably be helpful when configuring a dependency.Engine
// to run the result of Manifolds.
func IsFatal(err error) bool {
	return errors.Cause(err) == ErrRemoved
}

// WorstError will probably be helpful when configuring a dependency.Engine
// to run the result of Manifolds.
func WorstError(err, _ error) error {
	// Doesn't matter if there's only one fatal error.
	return err
}

// IgnoreErrRemoved returns nil if passed an error caused by ErrRemoved,
// and otherwise returns the original error.
func IgnoreErrRemoved(err error) error {
	cause := errors.Cause(err)
	if cause == ErrRemoved {
		return nil
	}
	return err
}
