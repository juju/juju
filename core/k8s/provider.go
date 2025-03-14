// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package k8s

import "github.com/juju/errors"

// WorkloadType defines a workload type on k8s.
type WorkloadType string

// Validate validates if this workload type is supported.
func (dt WorkloadType) Validate() error {
	if dt == "" {
		return nil
	}
	if dt == WorkloadTypeDeployment ||
		dt == WorkloadTypeStatefulSet ||
		dt == WorkloadTypeDaemonSet {
		return nil
	}
	return errors.NotSupportedf("workload type %q", dt)
}

const (
	// WorkloadTypeDeployment represents the "Deployment" workload type in k8s.
	WorkloadTypeDeployment WorkloadType = "Deployment"
	// WorkloadTypeStatefulSet represents the "StatefulSet" workload type in k8s.
	WorkloadTypeStatefulSet WorkloadType = "StatefulSet"
	// WorkloadTypeDaemonSet represents the "DaemonSet" workload type in k8s.
	WorkloadTypeDaemonSet WorkloadType = "DaemonSet"
)
