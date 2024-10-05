// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"errors"
	"strings"

	caasApplication "github.com/juju/juju/caas/kubernetes/provider/application"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs/envcontext"
)

var unsupportedConstraints = []string{
	constraints.Cores,
	constraints.VirtType,
	constraints.Container,
	constraints.InstanceType,
	constraints.Spaces,
	constraints.AllocatePublicIP,
	constraints.ImageID,
}

// ConstraintsValidator returns a Validator value which is used to
// validate and merge constraints.
func (k *kubernetesClient) ConstraintsValidator(ctx envcontext.ProviderCallContext) (constraints.Validator, error) {
	validator := constraints.NewValidator()
	validator.RegisterUnsupported(unsupportedConstraints)
	return &constraintsValidator{Validator: validator}, nil
}

type constraintsValidator struct {
	constraints.Validator
}

// Validate returns an error if the given constraints are not valid, and also
// any unsupported attributes.
func (v *constraintsValidator) Validate(cons constraints.Value) ([]string, error) {
	if cons.Tags != nil {

		var validateTopologyKeyTagFunc = func(key string, topologySpreadKey string, array []string) error {
			var topologySpreadValue, otherValue *string = nil, nil
			// Search for the topologySpreadValue for topologyKeyTag
			for tag := range array {
				if strings.HasPrefix(array[tag], topologySpreadKey) {
					trimmedValue := strings.TrimPrefix(array[tag], topologySpreadKey+"=")
					topologySpreadValue = &trimmedValue
					break
				}
			}
			if topologySpreadValue == nil {
				// Nothing to do, continue to the next check
				return nil
			}

			for tag := range array {
				if strings.HasPrefix(array[tag], key) {
					trimmedValue := strings.TrimPrefix(array[tag], key+"=")
					otherValue = &trimmedValue
					break
				}
			}

			if otherValue != nil && *topologySpreadValue == *otherValue {
				return errors.New("podAffinity/antiPodAffinity's topology-key and topologySpread's cannot have the same value")
			}
			return nil
		}

		positives, negatives := parseDelimitedValues(*cons.Tags)
		elements := append(positives, negatives...)

		topologySpreadKey := caasApplication.TopologySpreadPrefix + caasApplication.TopologyKeyTag
		podKey := caasApplication.PodPrefix + caasApplication.TopologyKeyTag
		if err := validateTopologyKeyTagFunc(podKey, topologySpreadKey, elements); err != nil {
			return nil, err
		}

		antiPodKey := caasApplication.AntiPodPrefix + caasApplication.TopologyKeyTag
		if err := validateTopologyKeyTagFunc(antiPodKey, topologySpreadKey, elements); err != nil {
			return nil, err
		}
	}

	return v.Validator.Validate(cons)
}

// parseDelimitedValues parses a slice of raw values coming from constraints
// (Tags or Spaces). The result is split into two slices - positives and
// negatives (prefixed with "^"). Empty values are ignored.
func parseDelimitedValues(rawValues []string) (positives, negatives []string) {
	for _, value := range rawValues {
		if value == "" || value == "^" {
			// Neither of these cases should happen in practise, as constraints
			// are validated before setting them and empty names for spaces or
			// tags are not allowed.
			continue
		}
		if strings.HasPrefix(value, "^") {
			negatives = append(negatives, strings.TrimPrefix(value, "^"))
		} else {
			positives = append(positives, value)
		}
	}
	return positives, negatives
}
