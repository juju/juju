// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

type ProvisioningOperation string

const (
	ScaleOperation         ProvisioningOperation = "scale"
	StorageUpdateOperation ProvisioningOperation = "storage-update"
)
