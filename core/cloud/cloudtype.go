// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

// CloudType represents a unique identifier for a Juju cloud provider.
type CloudType string

const (
	// CloudTypeAzure represents the Azure cloud provider.
	CloudTypeAzure CloudType = "azure"

	// CloudTypeAWS represents the AWS/EC2 cloud provider.
	CloudTypeEC2 CloudType = "ec2"

	// CloudTypeGCE represents the Google Compute Engine (GCE) cloud provider.
	CloudTypeGCE CloudType = "gce"

	// CloudTypeKubernetes represents the Kubernetes CAAS cloud provider.
	CloudTypeKubernetes CloudType = "kubernetes"

	// CloudTypeLXD represents the LXD cloud provider.
	CloudTypeLXD CloudType = "lxd"

	// CloudTypeManual represents the Manual cloud provider.
	CloudTypeManual CloudType = "manual"

	// CloudTypeMAAS represents the MAAS cloud provider.
	CloudTypeMAAS CloudType = "maas"

	// CloudTypeOCI represents the Oracle Cloud Infastructure (OCI) cloud
	// provider.
	CloudTypeOCI CloudType = "oci"

	// CloudTypeOpenStack represents the OpenStack cloud provider.
	CloudTypeOpenStack CloudType = "openstack"

	// CloudTypeVSphere represents the vSphere cloud provider.
	CloudTypevSphere CloudType = "vsphere"
)

// String returns the string representation of [CloudType]. This function
// fulfills the [fmt.Stringer] interface.
func (c CloudType) String() string {
	return string(c)
}
