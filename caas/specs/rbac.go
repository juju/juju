// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"github.com/juju/errors"
	rbacv1 "k8s.io/api/rbac/v1"
)

// ServiceAccountSpec defines spec for referencing to or creating a service account.
type ServiceAccountSpec struct {
	name                         string
	AutomountServiceAccountToken *bool    `yaml:"automountServiceAccountToken,omitempty"`
	ClusterRoleNames             []string `yaml:"ClusterRoleNames,omitempty"`

	Rules *rbacv1.PolicyRule `yaml:"rules,omitempty"` // TODO: still think we either need to model further or move rbac to k8specs level!!!!!!!!!!!!!
}

// GetName returns the service accout name.
func (sa ServiceAccountSpec) GetName() string {
	return sa.name
}

// SetName sets the service accout name.
func (sa *ServiceAccountSpec) SetName(name string) {
	sa.name = name
}

// Validate returns an error if the spec is not valid.
func (sa ServiceAccountSpec) Validate() error {
	if len(sa.ClusterRoleNames) == 0 && sa.Rules == nil {
		return errors.NotValidf("rules or clusterRoleNames are required")
	}
	return nil
}
