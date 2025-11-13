// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

// UnitK8sInformation represents the Kubernetes related information about a
// unit.
type UnitK8sInformation struct {
	// Addresses provides all of the available ip addresses for the unit include
	// the subnet mask of the address.
	Addresses  []string
	ProviderID string
	Ports      []string
	UnitName   string
}
