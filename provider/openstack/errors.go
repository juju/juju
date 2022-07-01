// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	gooseerrors "github.com/go-goose/goose/v5/errors"
	"github.com/juju/errors"

	"github.com/juju/juju/v2/environs/context"
	"github.com/juju/juju/v2/provider/common"
)

// handleCredentialError wraps the common handler,
// passing the Openstack-specific auth failure detection.
func handleCredentialError(err error, ctx context.ProviderCallContext) {
	common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
}

// IsAuthorisationFailure determines if the given error has an
// authorisation failure.
func IsAuthorisationFailure(err error) bool {
	// This should cover most cases.
	return gooseerrors.IsUnauthorised(errors.Cause(err))
}
