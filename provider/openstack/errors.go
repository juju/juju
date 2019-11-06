// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"github.com/juju/errors"
	gooseerrors "gopkg.in/goose.v2/errors"

	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/common"
)

// handleCredentialError wraps the common handler,
// passing the Openstack-specific auth failure detection.
func handleCredentialError(err error, ctx context.ProviderCallContext) {
	common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
}

// IsAuthorisationFailure determines if the given error has an authorisation failure.
func IsAuthorisationFailure(err error) bool {
	// This should cover most cases.
	if gooseerrors.IsUnauthorised(errors.Cause(err)) {
		return true
	}
	return false
}
