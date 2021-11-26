// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"github.com/juju/juju/environs/context"
)

const (
	// InstanceProfileAutoCreate defines the const value used for the constraint
	// when instance profile creation should be done on behalf of the user.
	InstanceProfileAutoCreate = "auto"
)

// InstanceRole defines the interface for environ providers to implement when
// they offer InstanceRole support for their respective cloud.
type InstanceRole interface {
	// CreateAutoInstanceRole is responsible for setting up an instance role on
	// behalf of the user.
	CreateAutoInstanceRole(context.ProviderCallContext, BootstrapParams) (string, error)

	// SupportsInstanceRoles indicates if Instance Roles are supported by this
	// environ.
	SupportsInstanceRoles(context.ProviderCallContext) bool
}
