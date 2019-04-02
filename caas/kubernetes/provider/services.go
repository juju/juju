// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	core "k8s.io/api/core/v1"
)

// // ControllerServiceTypeKey defines controller service type key.
// const ControllerServiceTypeKey = "controller-service-type"

var preferredControllerServiceTypes = map[string]core.ServiceType{
	K8sCloudAzure:    core.ServiceTypeLoadBalancer,
	K8sCloudCDK:      core.ServiceTypeLoadBalancer,
	K8sCloudEC2:      core.ServiceTypeLoadBalancer,
	K8sCloudGCE:      core.ServiceTypeLoadBalancer,
	K8sCloudMicrok8s: core.ServiceTypeClusterIP,
}
