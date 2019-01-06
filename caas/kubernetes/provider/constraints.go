// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs/context"
)

var unsupportedConstraints = []string{
	constraints.Cores,
	constraints.Tags,
	constraints.VirtType,
	constraints.Container,
	constraints.Arch,
	constraints.RootDisk,
	constraints.InstanceType,
	constraints.Spaces,
}

// ConstraintsValidator returns a Validator value which is used to
// validate and merge constraints.
func (k *kubernetesClient) ConstraintsValidator(ctx context.ProviderCallContext) (constraints.Validator, error) {
	validator := constraints.NewValidator()
	validator.RegisterUnsupported(unsupportedConstraints)
	return validator, nil
}
