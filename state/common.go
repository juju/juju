// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// This is a collection of common constants to be used in this
// package. Most of the juju packages access the state package,
// and it makes more sense to make some constants available
// at this package rather than the other way around.

const (
	K8sServiceTypeConfigKey = "kubernetes-service-type"

	// CAASProviderType is the provider type for k8s.
	CAASProviderType = "kubernetes"
)
