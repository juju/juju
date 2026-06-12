// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation

//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/state_mock.go github.com/juju/juju/internal/upgrades/upgradevalidation ModelAgentService,MachineService

var (
	CheckForDeprecatedUbuntuSeriesForModel = checkForDeprecatedUbuntuSeriesForModel
	GetCheckTargetVersionForModel          = getCheckTargetVersionForModel
	GetCheckForLXDVersion                  = getCheckForLXDVersion
)
