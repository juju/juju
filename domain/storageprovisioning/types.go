// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioning

import (
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
)

// ApplicationResourceTagInfo represents storage tag information in the context
// of an application.
type ApplicationResourceTagInfo struct {
	ApplicationName string
	ModelResourceTagInfo
}

// ModelResourceTagInfo represents storage tag information from the model.
type ModelResourceTagInfo struct {
	BaseResourceTags string
	ModelUUID        string
	ControllerUUID   string
}

// PlanDeviceType defines what type of storage attachment plan is required.
type PlanDeviceType int

const (
	// PlanDeviceTypeLocal indicates a local attachment.
	PlanDeviceTypeLocal PlanDeviceType = iota
	// PlanDeviceTypeISCSI indicates an iscsi attachment.
	PlanDeviceTypeISCSI
)

// ProvisionerIdent must have at one of two fields filled out to identify the
// provisioner interested in a storage entity.
type ProvisionerIdent struct {
	Model   model.UUID
	Machine machine.Name
}
