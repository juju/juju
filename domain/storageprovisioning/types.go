// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioning

type ResourceTagInfo struct {
	BaseResourceTags string
	ModelUUID        string
	ControllerUUID   string
	ApplicationName  string
}

// PlanDeviceType defines what type of storage attachment plan is required.
type PlanDeviceType int

const (
	// PlanDeviceTypeLocal indicates a local attachment.
	PlanDeviceTypeLocal = iota
	// PlanDeviceTypeISCSI indicates an iscsi attachment.
	PlanDeviceTypeISCSI
)
