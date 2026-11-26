// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package latest

import (
	v4_1_0 "github.com/juju/juju/domain/export/types/v4_1_0"
)

// ModelExport is the current target model-export payload type. It tracks the
// last entry of [github.com/juju/juju/domain/export.ExportVersions] / the
// version returned by export.LatestSupportedPayloadVersion.
type ModelExport = v4_1_0.ModelExport

// Model-export row type aliases for the latest schema version.
type (
	AgentStream                 = v4_1_0.AgentStream
	AgentVersion                = v4_1_0.AgentVersion
	Application                 = v4_1_0.Application
	ApplicationConstraint       = v4_1_0.ApplicationConstraint
	ApplicationController       = v4_1_0.ApplicationController
	Constraint                  = v4_1_0.Constraint
	ConstraintSpace             = v4_1_0.ConstraintSpace
	ConstraintTag               = v4_1_0.ConstraintTag
	ConstraintZone              = v4_1_0.ConstraintZone
	ContainerType               = v4_1_0.ContainerType
	ModelConfig                 = v4_1_0.ModelConfig
	ModelConstraint             = v4_1_0.ModelConstraint
	StorageFilesystem           = v4_1_0.StorageFilesystem
	StorageFilesystemAttachment = v4_1_0.StorageFilesystemAttachment
	StoragePool                 = v4_1_0.StoragePool
	StoragePoolAttribute        = v4_1_0.StoragePoolAttribute
	Unit                        = v4_1_0.Unit
)
