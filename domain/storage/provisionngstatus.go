// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

// ProvisioningStatus represents the status of a storage entity
// as recorded in the storage provisioning status lookup table.
type ProvisioningStatus int

const (
	ProvisioningStatusPending ProvisioningStatus = iota
	ProvisioningStatusProvisioned
	ProvisioningStatusError
)
