// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"github.com/juju/errors"
)

// PolicyRule defines rule spec for creating a role or cluster role.
type PolicyRule struct {
	Verbs     []string `yaml:"verbs"`
	APIGroups []string `yaml:"apiGroups,omitempty"`
	Resources []string `yaml:"resources,omitempty"`
}

// RBACSpec defines RBAC related spec.
type RBACSpec struct {
	AutomountServiceAccountToken *bool        `yaml:"automountServiceAccountToken,omitempty"`
	Global                       bool         `yaml:"global,omitempty"`
	Rules                        []PolicyRule `yaml:"rules,omitempty"`
}

// Validate returns an error if the spec is not valid.
func (rs RBACSpec) Validate() error {
	if len(rs.Rules) == 0 {
		return errors.NewNotValid(nil, "rules is required")
	}
	return nil
}

// ServiceAccountSpec defines spec for referencing or creating a service account.
type ServiceAccountSpec struct {
	name     string
	RBACSpec `yaml:",inline"`
}

// GetName returns the service accout name.
func (sa ServiceAccountSpec) GetName() string {
	return sa.name
}

// GetSpec returns the RBAC spec.
func (sa ServiceAccountSpec) GetSpec() RBACSpec {
	return sa.RBACSpec
}

// SetName sets the service accout name.
func (sa *ServiceAccountSpec) SetName(name string) {
	sa.name = name
}

// Validate returns an error if the spec is not valid.
func (sa ServiceAccountSpec) Validate() error {
	return errors.Trace(sa.RBACSpec.Validate())
}
