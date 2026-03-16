// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

// ProvisioningOperation represents the type of operation
// being performed on an application.
type ProvisioningOperation string

const (
	// NoOperation indicates no provisioning operation is in progress.
	NoOperation ProvisioningOperation = ""
	// ScaleOperation indicates a scaling operation is in progress.
	ScaleOperation ProvisioningOperation = "scale"
	// StorageUpdateOperation indicates a storage update operation is in progress.
	StorageUpdateOperation ProvisioningOperation = "storage-update"
)

// IsDifferentOperation reports whether the current operation differs from the
// requested operation.
func IsDifferentOperation(currentOp, requestedOp ProvisioningOperation) bool {
	return currentOp != NoOperation && currentOp != requestedOp
}
