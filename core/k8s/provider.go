// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package k8s

import "github.com/juju/errors"

// K8sDeploymentType defines a deployment type.
type K8sDeploymentType string

// Validate validates if this deployment type is supported.
func (dt K8sDeploymentType) Validate() error {
	if dt == "" {
		return nil
	}
	if dt == K8sDeploymentStateless ||
		dt == K8sDeploymentStateful ||
		dt == K8sDeploymentDaemon {
		return nil
	}
	return errors.NotSupportedf("deployment type %q", dt)
}

const (
	K8sDeploymentStateless K8sDeploymentType = "stateless"
	K8sDeploymentStateful  K8sDeploymentType = "stateful"
	K8sDeploymentDaemon    K8sDeploymentType = "daemon"
)
