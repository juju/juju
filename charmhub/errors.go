// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"github.com/juju/errors"

	"github.com/juju/juju/charmhub/transport"
)

// APIError extracts an API error from the given error or returns an error.
func APIError(err error) (transport.APIError, error) {
	if err == nil {
		return transport.APIError{}, nil
	}
	if IsAPIError(err) {
		return err.(transport.APIError), nil
	}
	return transport.APIError{}, errors.Annotatef(err, "not valid APIError")
}

// APIErrors extracts a slice of API errors from the given error or returns an
// error.
func APIErrors(err error) (transport.APIErrors, error) {
	if err == nil {
		return nil, nil
	}
	if IsAPIErrors(err) {
		return err.(transport.APIErrors), nil
	}
	return nil, errors.Annotatef(err, "not valid APIErrors")
}

// IsAPIError checks to see if the error is a valid API error.
func IsAPIError(err error) bool {
	_, ok := errors.Cause(err).(transport.APIError)
	return ok
}

// IsAPIErrors checks to see if the error is a valid series of API errors.
func IsAPIErrors(err error) bool {
	_, ok := errors.Cause(err).(transport.APIErrors)
	return ok
}

// Handle some of the basic error messages.
func handleBasicAPIErrors(list transport.APIErrors, logger Logger) error {
	if list == nil || len(list) == 0 {
		return nil
	}

	if errs, _ := APIErrors(list); errs != nil {
		masked := true
		defer func() {
			// Only log out the error if we're masking the original error, that
			// way you can at least find the issue in `debug-log`.
			// We do this because the original error message can be huge and
			// verbose, like a java stack trace!
			if masked {
				logger.Errorf("charmhub API error %d:%s", errs[0].Code, errs[0].Message)
			}
		}()

		switch errs[0].Code {
		case transport.ErrorCodeNotFound:
			return errors.NotFoundf("charm or bundle")
		case transport.ErrorCodeNameNotFound:
			return errors.NotFoundf("charm or bundle name")
		case transport.ErrorCodeResourceNotFound:
			return errors.NotFoundf("charm resource")
		case transport.ErrorCodeAPIError:
			return errors.Errorf("api error")
		case transport.ErrorCodeBadArgument:
			return errors.BadRequestf("query argument")
		}
		// We haven't handled the errors, so just return them.
		masked = false
		return errs
	}
	return list
}
