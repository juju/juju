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
		switch errs[0].Code {
		case transport.ErrorCodeNotFound, transport.ErrorCodeNameNotFound:
			return errors.NotFoundf(errs[0].Message)
		case transport.ErrorCodeAPIError:
			return errors.NotValidf(errs[0].Message)
		case transport.ErrorCodeBadArgument:
			// We never want to expose this to the user as it's very verbose and
			// is a developer error. We shouldn't leak implementation details
			// outside of the controller/juju.
			logger.Errorf("query argument not valid - message %s", errs[0].Message)
			return errors.NotValidf("query argument")
		}
		// We haven't handled the errors, so just return them.
		return errs
	}
	return list
}
