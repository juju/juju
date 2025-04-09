// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination services_mock_test.go github.com/juju/juju/apiserver/facades/client/application ExternalControllerService,NetworkService,StorageInterface,DeployFromRepository,BlockChecker,ModelConfigService,MachineService,ApplicationService,PortService,StubService,Leadership,StorageService,RelationService,ResourceService
//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination legacy_mock_test.go github.com/juju/juju/apiserver/facades/client/application Backend,Application,CaasBrokerInterface
//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination objectstore_mock_test.go github.com/juju/juju/core/objectstore ObjectStore
//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination storage_mock_test.go github.com/juju/juju/internal/storage ProviderRegistry
//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination facade_mock_test.go github.com/juju/juju/apiserver/facade Authorizer,Resources
//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination charm_mock_test.go github.com/juju/juju/internal/charm Charm,CharmMeta
//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination core_charm_mock_test.go github.com/juju/juju/core/charm Repository,RepositoryFactory

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
