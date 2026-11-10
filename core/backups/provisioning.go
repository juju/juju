// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

// InstanceMetadata records facts about the controller instance on which
// a backup is taken: its machine identifiers plus the substrate running
// it. The image fields are source pod references and may be tagged or
// digest-addressed.
//
// Every field is optional in the serialised form: machine controllers
// record only the machine identifiers, Kubernetes controllers also record
// the substrate facts.
type InstanceMetadata struct {
	// MachineID contains controller machine id from which this backup is taken.
	MachineID string `json:",omitempty"`
	// MachineInstanceID contains controller machine's instance id from
	// which this backup is taken.
	MachineInstanceID string `json:",omitempty"`

	// AgentImage is the container image running the controller API server.
	AgentImage string `json:",omitempty"`
	// CharmImage is the container image running the controller charm.
	CharmImage string `json:",omitempty"`
	// Architecture is the CPU architecture of the node hosting the
	// controller.
	Architecture string `json:",omitempty"`
	// StorageClass is the storage class of the controller's data volume.
	StorageClass string `json:",omitempty"`
	// StorageCapacityMiB is the controller data volume capacity in
	// mebibytes, rounded up to the next whole mebibyte.
	StorageCapacityMiB uint64 `json:",omitempty"`
	// ServiceType is the controller service exposure mode, using the
	// caas.ServiceType values "cluster", "loadbalancer" or "external".
	ServiceType string `json:",omitempty"`
	// ExternalName is the DNS name of an external name service.
	ExternalName string `json:",omitempty"`
	// ExternalIPs are additional external IP addresses of the service.
	ExternalIPs []string `json:",omitempty"`
	// LoadBalancerIP is the requested IP of a load balancer service.
	LoadBalancerIP string `json:",omitempty"`
	// ServiceAnnotations are the annotations of the controller service.
	ServiceAnnotations map[string]string `json:",omitempty"`
}
