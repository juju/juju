// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeservices

//go:generate go run go.uber.org/mock/mockgen -typed -package upgradeservices -destination servicefactory_mock_test.go github.com/juju/juju/internal/services UpgradeServices,UpgradeServicesGetter
//go:generate go run go.uber.org/mock/mockgen -typed -package upgradeservices -destination changestream_mock_test.go github.com/juju/juju/core/changestream WatchableDBGetter
