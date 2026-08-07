// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

var (
	FinalizePodBootstrapConfig = finalizePodBootstrapConfig
	ValidateUploadAllowed      = validateUploadAllowed
	GetBootstrapToolsVersion   = getBootstrapToolsVersion
	FindTools                  = &findTools
	FindBootstrapTools         = findBootstrapTools
	FindPackagedTools          = findPackagedTools
	AcquireControllerSnap      = &acquireControllerSnap
	SnapArch                   = snapArch
	BuildCommandFunc           = &buildCommandFunc
	LookPathFunc               = &lookPathFunc
)

// AcquiredSnap is a test-only alias for the unexported acquiredControllerSnap,
// so external test packages can construct and assert on acquisition results.
type AcquiredSnap = acquiredControllerSnap
