// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"github.com/juju/errors"
	"github.com/juju/utils/featureflag"

	"github.com/juju/juju/feature"
)

// loggedErrorStack is a developer helper function that will cause the error
// stack of the error to be printed out at error severity if and only if the
// "log-error-stack" feature flag has been specified.  The passed in error
// is also the return value of this function.
func loggedErrorStack(err error) error {
	if featureflag.Enabled(feature.LogErrorStack) {
		logger.Errorf("error stack:\n%s", errors.ErrorStack(err))
	}
	return err
}
