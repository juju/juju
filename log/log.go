// Copyright 2011-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package log

import (
	"fmt"

	"github.com/juju/loggo"
)

// Log the error and return an error with the same text.
func LoggedErrorf(logger loggo.Logger, format string, a ...interface{}) error {
	logger.Logf(loggo.ERROR, format, a...)
	return fmt.Errorf(format, a...)
}
