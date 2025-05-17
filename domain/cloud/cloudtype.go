// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

// CloudType represents the provider type of a cloud.
type CloudType int

const (
	// CloudTypeKubernetes represents the Kubernetes CAAS provider.
	CloudTypeKubernetes CloudType = iota

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

	// CloudTypevSphere represents the vSphere cloud provider.
	CloudTypevSphere
)
