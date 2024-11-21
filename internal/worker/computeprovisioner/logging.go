// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package computeprovisioner

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/featureflag"
)

// loggedErrorStack is a developer helper function that will cause the error
// stack of the error to be printed out at error severity if and only if the
// "log-error-stack" feature flag has been specified.  The passed in error
// is also the return value of this function.
func loggedErrorStack(logger logger.Logger, err error) error {
	if featureflag.Enabled(featureflag.LogErrorStack) {
		logger.Errorf(context.TODO(), "error stack:\n%s", errors.ErrorStack(err))
	}
	return err
}
