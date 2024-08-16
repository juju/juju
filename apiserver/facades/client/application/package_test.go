// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	stdtesting "testing"

	"github.com/juju/juju/internal/testing"
)

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/lxdprofile_mock.go github.com/juju/juju/internal/charm LXDProfiler
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/state_mock.go github.com/juju/juju/state StorageAttachment,StorageInstance,MachinePortRanges,UnitPortRanges,CloudContainer
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/storage_mock.go github.com/juju/juju/internal/storage ProviderRegistry
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/leadership_mock.go github.com/juju/juju/core/leadership Reader
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/application_mock.go github.com/juju/juju/apiserver/facades/client/application Backend,StorageInterface,BlockChecker,Model,CaasBrokerInterface,Application,RemoteApplication,Charm,Relation,Unit,RelationUnit,Machine,Bindings,Resources
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/offer_mock.go github.com/juju/juju/apiserver/facades/client/application OfferConnection
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/objectstore_mock.go github.com/juju/juju/core/objectstore ObjectStore
//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination deployrepository_mocks_test.go github.com/juju/juju/apiserver/facades/client/application Bindings,DeployFromRepositoryState,DeployFromRepositoryValidator,Model,Machine,Application,Charm
//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination repository_mocks_test.go github.com/juju/juju/core/charm Repository,RepositoryFactory
//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination service_mock_test.go github.com/juju/juju/apiserver/facades/client/application MachineService,ApplicationService,StoragePoolGetter,NetworkService,ExternalControllerService,ModelConfigService,ModelAgentService
//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination environ_mock_test.go github.com/juju/juju/environs InstancePrechecker
