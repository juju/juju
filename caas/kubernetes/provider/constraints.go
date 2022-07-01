// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"github.com/juju/juju/v2/core/constraints"
	"github.com/juju/juju/v2/environs/context"
)

var unsupportedConstraints = []string{
	constraints.Cores,
	constraints.VirtType,
	constraints.Container,
	constraints.InstanceType,
	constraints.Spaces,
	constraints.AllocatePublicIP,
}

// ConstraintsValidator returns a Validator value which is used to
// validate and merge constraints.
func (k *kubernetesClient) ConstraintsValidator(ctx context.ProviderCallContext) (constraints.Validator, error) {
	validator := constraints.NewValidator()
	validator.RegisterUnsupported(unsupportedConstraints)
	return validator, nil
}
