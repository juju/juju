// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"
	"io"

	"github.com/juju/juju/core/logger"
)

// closeAndLog calls the closer's Close() and logs any error returned therefrom.
func closeAndLog(closer io.Closer, label string, logger logger.Logger) {
	if closer == nil {
		return
	}
	if err := closer.Close(); err != nil {
		logger.Errorf(context.TODO(), "while closing %s: %v", label, err)
	}
}
