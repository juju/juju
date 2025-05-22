// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/state_mock.go github.com/juju/juju/internal/upgrades/upgradevalidation State,ModelAgentService

var (
	CheckForDeprecatedUbuntuSeriesForModel = checkForDeprecatedUbuntuSeriesForModel
	GetCheckTargetVersionForModel          = getCheckTargetVersionForModel
	GetCheckForLXDVersion                  = getCheckForLXDVersion
)
