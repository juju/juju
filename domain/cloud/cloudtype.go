// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

// CloudType represents the provider type of a cloud.
type CloudType int

const (
	// cloudTypeInvalidLow is a sentinel value used to indicate the lower
	// invalid bounds for [CloudType] values.
	//
	// This value must only ever be used in validating a [CloudType].
	cloudTypeInvalidLow CloudType = iota - 1

	// CloudTypeKubernetes represents the Kubernetes CAAS provider.
	CloudTypeKubernetes

	// CloudTypeLXD represents the LXD cloud provider.
	CloudTypeLXD

	// CloudTypeMAAS represents the MAAS cloud provider.
	CloudTypeMAAS

	// CloudTypeManual represents the Manual cloud provider.
	CloudTypeManual

	// CloudTypeAzure represents the Azure cloud provider.
	CloudTypeAzure

	// CloudTypeEC2 represents the AWS/EC2 cloud provider.
	CloudTypeEC2

	// CloudTypeGCE represents the Google Compute Engine (GCE) cloud provider.
	CloudTypeGCE

	// CloudTypeOCI represents the Oracle Cloud Infrastructure (OCI) cloud
	// provider.
	CloudTypeOCI

	// CloudTypeOpenStack represents the OpenStack cloud provider.
	CloudTypeOpenStack

	// CloudTypeVSphere represents the vSphere cloud provider.
	CloudTypeVSphere

	// cloudTypeInvalidHigh is a sentinel value used to indicate the upper
	// invalid bounds for [CloudType] values.
	//
	// This value must only ever be used in validating a [CloudType].
	cloudTypeInvalidHigh
)

// IsValid provides a quick reference way to check that the value in [CloudType]
// is valid and understood by Juju.
func (ct CloudType) IsValid() bool {
	return ct > cloudTypeInvalidLow && ct < cloudTypeInvalidHigh
}

// String returns a string representation of [CloudType]. The string
// representation aligns with the unique lowercase string identifiers used
// throughout Juju.
//
// If the value in CloudType is not understand an empty string is returned. If
// the caller would like to know ahead of time if the value is valid then use
// [CloudType.IsValid].
//
// String implements the [fmt.Stringer] interface.
func (ct CloudType) String() string {
	switch ct {
	case CloudTypeKubernetes:
		return "kubernetes"
	case CloudTypeLXD:
		return "lxd"
	case CloudTypeMAAS:
		return "maas"
	case CloudTypeManual:
		return "manual"
	case CloudTypeAzure:
		return "azure"
	case CloudTypeEC2:
		return "ec2"
	case CloudTypeGCE:
		return "gce"
	case CloudTypeOCI:
		return "oci"
	case CloudTypeOpenStack:
		return "openstack"
	case CloudTypeVSphere:
		return "vsphere"
	}
	return ""
}
