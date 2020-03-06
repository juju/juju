// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"github.com/juju/errors"
)

// PolicyRule defines a rule policy for a role or cluster role.
type PolicyRule struct {
	Verbs           []string `json:"verbs" yaml:"verbs"`
	APIGroups       []string `json:"apiGroups,omitempty" yaml:"apiGroups,omitempty"`
	Resources       []string `json:"resources,omitempty" yaml:"resources,omitempty"`
	ResourceNames   []string `json:"resourceNames,omitempty" yaml:"resourceNames,omitempty"`
	NonResourceURLs []string `json:"nonResourceURLs,omitempty" yaml:"nonResourceURLs,omitempty"`
}

// PrimeRBACSpec defines RBAC related spec.
type PrimeRBACSpec struct {
	AutomountServiceAccountToken *bool        `yaml:"automountServiceAccountToken,omitempty"`
	Global                       bool         `yaml:"global,omitempty"`
	Rules                        []PolicyRule `yaml:"rules,omitempty"`
}

// Validate returns an error if the spec is not valid.
func (rs PrimeRBACSpec) Validate() error {
	if len(rs.Rules) == 0 {
		return errors.NewNotValid(nil, "rules is required")
	}
	return nil
}

// PrimeServiceAccountSpec defines spec for referencing or creating RBAC resource for the application.
type PrimeServiceAccountSpec struct {
	name          string
	PrimeRBACSpec `yaml:",inline"`
}

// GetName returns the service accout name.
func (sa PrimeServiceAccountSpec) GetName() string {
	return sa.name
}

// GetSpec returns the RBAC spec.
func (sa PrimeServiceAccountSpec) GetSpec() PrimeRBACSpec {
	return sa.PrimeRBACSpec
}

// SetName sets the service accout name.
func (sa *PrimeServiceAccountSpec) SetName(name string) {
	sa.name = name
}

// Validate returns an error if the spec is not valid.
func (sa PrimeServiceAccountSpec) Validate() error {
	return errors.Trace(sa.PrimeRBACSpec.Validate())
}
