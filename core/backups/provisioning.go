// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

// KubernetesProvisioning records the running controller's substrate. Images
// are source pod references and may be tagged or digest-addressed.
type KubernetesProvisioning struct {
	// AgentImage is the container image running the controller API server.
	AgentImage string
	// CharmImage is the container image running the controller charm.
	CharmImage string
	// Architecture is the CPU architecture of the node hosting the
	// controller.
	Architecture string
	// StorageClass is the storage class of the controller's data volume.
	StorageClass string
	// StorageSizeMiB is the controller data volume capacity in mebibytes,
	// rounded up to the next whole mebibyte.
	StorageSizeMiB uint64
	// ServiceType is the controller service exposure mode, using the
	// caas.ServiceType values "cluster", "loadbalancer" or "external".
	ServiceType string
	// ExternalName is the DNS name of an external name service.
	ExternalName string
	// ExternalIPs are additional external IP addresses of the service.
	ExternalIPs []string
	// LoadBalancerIP is the requested IP of a load balancer service.
	LoadBalancerIP string
	// ServiceAnnotations are the annotations of the controller service.
	ServiceAnnotations map[string]string
}
