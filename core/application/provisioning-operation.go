// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

type ProvisioningOperation string

const (
	NoOperation            ProvisioningOperation = ""
	ScaleOperation         ProvisioningOperation = "scale"
	StorageUpdateOperation ProvisioningOperation = "storage-update"
)
