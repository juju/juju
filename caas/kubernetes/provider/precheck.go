// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/environs"
)

// PrecheckInstance performs a preflight check on the specified
// series and constraints, ensuring that they are possibly valid for
// creating an instance in this model.
//
// PrecheckInstance is best effort, and not guaranteed to eliminate
// all invalid parameters. If PrecheckInstance returns nil, it is not
// guaranteed that the constraints are valid; if a non-nil error is
// returned, then the constraints are definitely invalid.
func (k *kubernetesClient) PrecheckInstance(ctx context.Context, params environs.PrecheckInstanceParams) error {
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

	if params.Placement != "" {
		return errors.NotValidf("placement directive %q", params.Placement)
	}
	if params.Constraints.Tags == nil {
		return nil
	}
	affinityLabels := *params.Constraints.Tags
	labelsString := strings.Join(affinityLabels, ",")
	for _, labelPair := range affinityLabels {
		parts := strings.Split(labelPair, "=")
		if len(parts) != 2 {
			return errors.Errorf("invalid node affinity constraints: %v", labelsString)
		}
		key := strings.Trim(parts[0], " ")
		if strings.HasPrefix(key, "^") {
			if len(key) == 1 {
				return errors.Errorf("invalid node affinity constraints: %v", labelsString)
			}
		}
	}
	return nil
}
