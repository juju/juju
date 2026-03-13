// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

type ProvisioningOperation string

const (
	NoOperation            ProvisioningOperation = ""
	ScaleOperation         ProvisioningOperation = "scale"
	StorageUpdateOperation ProvisioningOperation = "storage-update"
)

// IsDifferentOperation reports whether the current operation differs from the
// requested operation.
func IsDifferentOperation(currentOp, requestedOp ProvisioningOperation) bool {
	return currentOp != NoOperation && currentOp != requestedOp
}
