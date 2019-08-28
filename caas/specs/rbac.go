// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"github.com/juju/errors"
	rbacv1 "k8s.io/api/rbac/v1"
)

// RBACType describes Role/Binding type.
type RBACType string

const (
	// Role is the single namespace based.
	Role RBACType = "Role"
	// ClusterRole is the all namespaces based.
	ClusterRole RBACType = "ClusterRole"

	// RoleBinding is the single namespace based.
	RoleBinding RBACType = "RoleBinding"
	// ClusterRoleBinding is the all namespaces based.
	ClusterRoleBinding RBACType = "ClusterRoleBinding"
)

var (
	// RoleTypes defines supported role types.
	RoleTypes = []RBACType{
		Role, ClusterRole,
	}
	// RoleBindingTypes defines supported role binding types.
	RoleBindingTypes = []RBACType{
		RoleBinding, ClusterRoleBinding,
	}
)

// RoleSpec defines config for referencing to or creating a Role or Cluster resource.
type RoleSpec struct {
	Name string   `yaml:"name"`
	Type RBACType `yaml:"type"`
	// We assume this is an existing Role/ClusterRole if Rules is empty.
	// Existing Role/ClusterRoles can be only referenced but will never be deleted or updated by Juju.
	// Role/ClusterRoles are created by Juju will be properly labeled.
	Rules []rbacv1.PolicyRule `yaml:"rules"`
}

// Validate returns an error if the spec is not valid.
func (r RoleSpec) Validate() error {
	for _, v := range RoleTypes {
		if r.Type == v {
			return nil
		}
	}
	return errors.NotSupportedf("%q", r.Type)
}

// RoleBindingSpec defines config for referencing to or creating a RoleBinding or ClusterRoleBinding resource.
type RoleBindingSpec struct {
	Name string   `yaml:"name"`
	Type RBACType `yaml:"type"`
}

// Validate returns an error if the spec is not valid.
func (rb RoleBindingSpec) Validate() error {
	for _, v := range RoleBindingTypes {
		if rb.Type == v {
			return nil
		}
	}
	return errors.NotSupportedf("%q", rb.Type)
}

// Capabilities defines RBAC related config.
type Capabilities struct {
	Role        *RoleSpec        `yaml:"role"`
	RoleBinding *RoleBindingSpec `yaml:"roleBinding"`
}

// Validate returns an error if the spec is not valid.
func (c Capabilities) Validate() error {
	if c.Role == nil {
		return errors.New("role is required for capabilities")
	}
	if c.RoleBinding == nil {
		return errors.New("roleBinding is required for capabilities")
	}
	if err := c.Role.Validate(); err != nil {
		return errors.Trace(err)
	}
	if err := c.RoleBinding.Validate(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// ServiceAccountSpec defines spec for referencing to or creating a service account.
type ServiceAccountSpec struct {
	Name                         string        `yaml:"name"`
	AutomountServiceAccountToken *bool         `yaml:"automountServiceAccountToken,omitempty"`
	Capabilities                 *Capabilities `yaml:"capabilities"`
}

// Validate returns an error if the spec is not valid.
func (sa ServiceAccountSpec) Validate() error {
	if sa.Capabilities == nil {
		return errors.New("capabilities is required for service account")
	}
	if err := sa.Capabilities.Validate(); err != nil {
		return errors.Trace(err)
	}
	return nil
}
