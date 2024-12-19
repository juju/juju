// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"strings"

	"github.com/juju/errors"

	caasApplication "github.com/juju/juju/caas/kubernetes/provider/application"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs/context"
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
func (k *kubernetesClient) ConstraintsValidator(ctx context.ProviderCallContext) (constraints.Validator, error) {
	validator := constraints.NewValidator()
	validator.RegisterUnsupported(unsupportedConstraints)
	return &constraintsValidator{Validator: validator}, nil
}

type constraintsValidator struct {
	constraints.Validator
}

// validateTopologyKeyTag checks if the topologyKeyTag is the same as the topologySpreadKey
func validateTopologyKeyTag(key string, topologySpreadKey string, tagList []string) error {
	var topologySpreadValue, otherValue *string
	// Search for the topologySpreadValue for topologyKeyTag
	for tag := range tagList {
		if strings.HasPrefix(tagList[tag], topologySpreadKey) {
			trimmedValue := strings.TrimPrefix(tagList[tag], topologySpreadKey+"=")
			topologySpreadValue = &trimmedValue
			break
		}
	}
	if topologySpreadValue == nil {
		// Nothing to do, continue to the next check
		return nil
	}

	for tag := range tagList {
		if strings.HasPrefix(tagList[tag], key) {
			trimmedValue := strings.TrimPrefix(tagList[tag], key+"=")
			otherValue = &trimmedValue
			break
		}
	}

	if otherValue != nil && *topologySpreadValue == *otherValue {
		return errors.New("If both are specified, the topology-key for (anti) pod affinity and topology-spread cannot have the same value")
	}
	return nil
}

// Validate returns an error if the given constraints are not valid, and also
// any unsupported attributes.
func (v *constraintsValidator) Validate(cons constraints.Value) ([]string, error) {
	validated, err := v.Validator.Validate(cons)
	if err != nil {
		return nil, err
	}
	if cons.Tags == nil {
		return nil, nil
	}
	positives, negatives := parseDelimitedValues(*cons.Tags)
	elements := append(positives, negatives...)

	topologySpreadKey := caasApplication.TopologySpreadPrefix + caasApplication.TopologyKeyTag
	podKey := caasApplication.PodPrefix + caasApplication.TopologyKeyTag
	if err := validateTopologyKeyTag(podKey, topologySpreadKey, elements); err != nil {
		return nil, err
	}

	antiPodKey := caasApplication.AntiPodPrefix + caasApplication.TopologyKeyTag
	if err := validateTopologyKeyTag(antiPodKey, topologySpreadKey, elements); err != nil {
		return nil, err
	}

	return validated, nil
}

// parseDelimitedValues parses a slice of raw values coming from tag constraints
// The result is split into two slices - positives and negatives (prefixed with "^"). Empty values are ignored.
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
