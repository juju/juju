// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/context_mock.go github.com/juju/juju/internal/upgrades Context

var UpgradeOperations = &upgradeOperations
