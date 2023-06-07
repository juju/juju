// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"github.com/juju/errors"
)

const (
	// ProvisioningStateInconsistent is returned by SetProvisioningState when the provisioning state
	// is inconsistent with the application scale.
	ProvisioningStateInconsistent = errors.ConstError("provisioning state is inconsistent")

	// ErrApplicationShouldNotHaveUnits is returned by SetCharm when the application has units when
	// it is expected to not have units. Used for upgrading from podspec to sidecar charms.
	ErrApplicationShouldNotHaveUnits = errors.ConstError("application should not have units")
)
