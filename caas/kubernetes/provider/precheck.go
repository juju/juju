// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/utils/keyvalues"
)

// PrecheckInstance performs a preflight check on the specified
// series and constraints, ensuring that they are possibly valid for
// creating an instance in this model.
//
// PrecheckInstance is best effort, and not guaranteed to eliminate
// all invalid parameters. If PrecheckInstance returns nil, it is not
// guaranteed that the constraints are valid; if a non-nil error is
// returned, then the constraints are definitely invalid.
func (k *kubernetesClient) PrecheckInstance(ctx context.ProviderCallContext, params environs.PrecheckInstanceParams) error {
	// Ensure there are no unsupported constraints.
	// Clouds generally don't enforce this but we will
	// for Kubernetes deployments.
	validator, err := k.ConstraintsValidator(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	unsupported, err := validator.Validate(params.Constraints)
	if err != nil {
		return errors.NotValidf("constraints %q", params.Constraints.String())
	}
	if len(unsupported) > 0 {
		return errors.NotSupportedf("constraints %v", strings.Join(unsupported, ","))
	}

	if params.Series != "kubernetes" {
		return errors.NotValidf("series %q", params.Series)
	}

	if params.Placement == "" {
		return nil
	}

	// Check placement is valid.
	// TODO(caas) - check for valid node labels?
	// Placement is a comma separated list of key-value pairs (node labels).
	_, err = keyvalues.Parse(strings.Split(params.Placement, ","), false)
	if err != nil {
		return errors.NotValidf("placement directive %q", params.Placement)
	}
	return nil
}
