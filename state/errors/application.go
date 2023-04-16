// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"github.com/juju/errors"
)

const (
	ProvisioningStateInconsistent = errors.ConstError("provisioning state is inconsistent")
)
