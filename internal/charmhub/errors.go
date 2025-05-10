// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"

	"github.com/juju/errors"

	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/charmhub/transport"
)

// Handle some of the basic error messages.
func handleBasicAPIErrors(ctx context.Context, list transport.APIErrors, logger corelogger.Logger) error {
	if len(list) == 0 {
		return nil
	}

	masked := true
	defer func() {
		// Only log out the error if we're masking the original error, that
		// way you can at least find the issue in `debug-log`.
		// We do this because the original error message can be huge and
		// verbose, like a java stack trace!
		if masked {
			logger.Errorf(ctx, "charmhub API error %s:%s", list[0].Code, list[0].Message)
		}
	}()

	switch list[0].Code {
	case transport.ErrorCodeNotFound:
		return errors.NotFoundf("charm or bundle")
	case transport.ErrorCodeNameNotFound:
		return errors.NotFoundf("charm or bundle name")
	case transport.ErrorCodeResourceNotFound:
		return errors.NotFoundf("charm resource")
	case transport.ErrorCodeAPIError:
		return errors.Errorf("unexpected api error attempting to query charm or bundle from the charmhub store")
	case transport.ErrorCodeBadArgument:
		return errors.BadRequestf("query argument")
	}
	// We haven't handled the errors, so just return them.
	masked = false
	return list
}
