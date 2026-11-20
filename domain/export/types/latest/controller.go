// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package latest

import (
	ctrlv4_1_0 "github.com/juju/juju/domain/export/types/controller/v4_1_0"
)

// ControllerExport is the current target controller-export payload type. It
// tracks the latest controller export version in
// [github.com/juju/juju/domain/export].
type ControllerExport = ctrlv4_1_0.ControllerExport

// Controller-export row type aliases for the latest schema version.
type (
	Cloud                   = ctrlv4_1_0.Cloud
	CloudRegion             = ctrlv4_1_0.CloudRegion
	CloudType               = ctrlv4_1_0.CloudType
	Controller              = ctrlv4_1_0.Controller
	ControllerConfig        = ctrlv4_1_0.ControllerConfig
	ControllerSshServerPort = ctrlv4_1_0.ControllerSshServerPort
	Model                   = ctrlv4_1_0.Model
	ModelType               = ctrlv4_1_0.ModelType
)
