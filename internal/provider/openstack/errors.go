// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	gooseerrors "github.com/go-goose/goose/v5/errors"
	"github.com/juju/errors"
)

// IsAuthorisationFailure determines if the given error has an
// authorisation failure.
func IsAuthorisationFailure(err error) bool {
	// This should cover most cases.
	return gooseerrors.IsUnauthorised(errors.Cause(err))
}

func isNotFoundError(err error) bool {
	return errors.Is(err, errors.NotFound) || gooseerrors.IsNotFound(errors.Cause(err))
}
