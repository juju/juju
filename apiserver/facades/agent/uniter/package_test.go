// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package uniter -destination clock_mock_test.go github.com/juju/clock Clock
//go:generate go run go.uber.org/mock/mockgen -typed -package uniter_test -destination package_mocks_test.go github.com/juju/juju/apiserver/facades/agent/uniter LXDProfileBackend,LXDProfileMachine,LXDProfileUnit,LXDProfileBackendV2,LXDProfileMachineV2,LXDProfileUnitV2,LXDProfileCharmV2
//go:generate go run go.uber.org/mock/mockgen -typed -package uniter -destination secret_mocks_test.go github.com/juju/juju/apiserver/facades/agent/uniter SecretService
//go:generate go run go.uber.org/mock/mockgen -typed -package uniter -destination leadership_mocks_test.go github.com/juju/juju/core/leadership Checker,Token
//go:generate go run go.uber.org/mock/mockgen -typed -package uniter_test -destination legacy_service_mock_test.go github.com/juju/juju/apiserver/facades/agent/uniter ModelConfigService,ModelInfoService,NetworkService,MachineService
//go:generate go run go.uber.org/mock/mockgen -typed -package uniter_test -destination facade_mock_test.go github.com/juju/juju/apiserver/facade WatcherRegistry
//go:generate go run go.uber.org/mock/mockgen -typed -package uniter -destination service_mock_test.go github.com/juju/juju/apiserver/facades/agent/uniter ApplicationService,RelationService,ModelInfoService
//go:generate go run go.uber.org/mock/mockgen -typed -package uniter -destination watcher_registry_mock_test.go github.com/juju/juju/apiserver/facade WatcherRegistry

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}
