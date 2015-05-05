// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/featureflag"

	"github.com/juju/juju/feature"
)

var logger = loggo.GetLogger("juju.utils")

// LoggedErrorStack is a developer helper function that will cause the error
// stack of the error to be printed out at error severity if and only if the
// "log-error-stack" feature flag has been specified.  The passed in error
// is also the return value of this function.
func LoggedErrorStack(err error) error {
	if featureflag.Enabled(feature.LogErrorStack) {
		logger.Errorf("error stack:\n%s", errors.ErrorStack(err))
	}
	return err
}
