// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Plan constructs are copied directly from the Pebble code base to Juju without
// any modification. We perform this copy as the Pebble package does not isolate
// plan types fully and will cause Juju to not compile correctly on systems that
// are not Linux based.
//
// The code can be found at https://github.com/canonical/pebble/internal/plan
//
// Do not modify this file. Instead propose a change against Pebble and then
// copy the changes over.
package plan

import (
	"fmt"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

type OptionalDuration struct {
	Value time.Duration
	IsSet bool
}

func (o OptionalDuration) IsZero() bool {
	return !o.IsSet
}

func (o OptionalDuration) MarshalYAML() (interface{}, error) {
	if !o.IsSet {
		return nil, nil
	}
	return o.Value.String(), nil
}

func (o *OptionalDuration) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.ScalarNode {
		return fmt.Errorf("duration must be a YAML string")
	}
	duration, err := time.ParseDuration(value.Value)
	if err != nil {
		return fmt.Errorf("invalid duration %q", value.Value)
	}
	o.Value = duration
	o.IsSet = true
	return nil
}

type OptionalFloat struct {
	Value float64
	IsSet bool
}

func (o OptionalFloat) IsZero() bool {
	return !o.IsSet
}

func (o OptionalFloat) MarshalYAML() (interface{}, error) {
	if !o.IsSet {
		return nil, nil
	}
	return o.Value, nil
}

func (o *OptionalFloat) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.ScalarNode {
		return fmt.Errorf("value must be a YAML number")
	}
	n, err := strconv.ParseFloat(value.Value, 64)
	if err != nil {
		return fmt.Errorf("invalid floating-point number %q", value.Value)
	}
	o.Value = n
	o.IsSet = true
	return nil
}
