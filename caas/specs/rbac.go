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

// ServiceAccountSpecV2 defines spec for referencing or creating RBAC resource for the application for version 2.
type ServiceAccountSpecV2 struct {
	AutomountServiceAccountToken *bool        `json:"automountServiceAccountToken,omitempty" yaml:"automountServiceAccountToken,omitempty"`
	Global                       bool         `json:"global,omitempty" yaml:"global,omitempty"`
	Rules                        []PolicyRule `json:"rules,omitempty" yaml:"rules,omitempty"`
}

// Validate returns an error if the spec is not valid.
func (sa ServiceAccountSpecV2) Validate() error {
	if len(sa.Rules) == 0 {
		return errors.NewNotValid(nil, "rules is required")
	}
	return nil
}

// ToLatest converts ServiceAccountSpecV2 to the latest version.
func (sa ServiceAccountSpecV2) ToLatest() *PrimeServiceAccountSpecV3 {
	return &PrimeServiceAccountSpecV3{
		ServiceAccountSpecV3: ServiceAccountSpecV3{
			AutomountServiceAccountToken: sa.AutomountServiceAccountToken,
			Roles: []Role{
				{
					Global: sa.Global,
					Rules:  sa.Rules,
				},
			},
		},
	}
}

// Role defines role spec for version 3.
type Role struct {
	Name   string       `json:"name" yaml:"name"`
	Global bool         `json:"global,omitempty" yaml:"global,omitempty"`
	Rules  []PolicyRule `json:"rules,omitempty" yaml:"rules,omitempty"`
}

// Validate returns an error if the spec is not valid.
func (r Role) Validate() error {
	if len(r.Rules) == 0 {
		return errors.NewNotValid(nil, "rules is required")
	}
	return nil
}

// ServiceAccountSpecV3 defines spec for creating RBAC resource for the application for version 3.
type ServiceAccountSpecV3 struct {
	AutomountServiceAccountToken *bool `json:"automountServiceAccountToken,omitempty" yaml:"automountServiceAccountToken,omitempty"`

	Roles []Role `json:"roles" yaml:"roles"`
}

// Validate returns an error if the spec is not valid.
func (sa ServiceAccountSpecV3) Validate() error {
	if len(sa.Roles) == 0 {
		return errors.NewNotValid(nil, "roles is required")
	}
	for _, r := range sa.Roles {
		if err := r.Validate(); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// PrimeServiceAccountSpecV3 defines spec for creating the prime RBAC resources for version 3.
type PrimeServiceAccountSpecV3 struct {
	name                 string
	ServiceAccountSpecV3 `json:",inline" yaml:",inline"`
}

// GetName returns the service accout name.
func (psa PrimeServiceAccountSpecV3) GetName() string {
	return psa.name
}

// SetName sets the service accout name.
func (psa *PrimeServiceAccountSpecV3) SetName(name string) {
	psa.name = name
	for i, r := range psa.Roles {
		r.Name = name
		psa.Roles[i] = r
	}
}

// Validate returns an error if the spec is not valid.
func (psa PrimeServiceAccountSpecV3) Validate() error {
	if err := psa.ServiceAccountSpecV3.Validate(); err != nil {
		return errors.Trace(err)
	}
	if len(psa.Roles) > 1 {
		return errors.NewNotValid(nil, "the prime service can only have one role or cluster role")
	}
	return nil
}
