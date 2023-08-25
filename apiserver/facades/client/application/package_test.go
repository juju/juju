// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/lxdprofile_mock.go github.com/juju/charm/v11 LXDProfiler
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/state_mock.go github.com/juju/juju/state StorageAttachment,StorageInstance,MachinePortRanges,UnitPortRanges,CloudContainer
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/state_storage_mock.go github.com/juju/juju/state/storage Storage
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/storage_mock.go github.com/juju/juju/internal/storage ProviderRegistry
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/poolmanager_mock.go github.com/juju/juju/internal/storage/poolmanager PoolManager
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/leadership_mock.go github.com/juju/juju/core/leadership Reader
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/application_mock.go github.com/juju/juju/apiserver/facades/client/application Backend,StorageInterface,BlockChecker,Model,CaasBrokerInterface,Application,RemoteApplication,Charm,Relation,Unit,RelationUnit,Machine,Generation,Bindings,Resources,ECService
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/offer_mock.go github.com/juju/juju/apiserver/facades/client/application OfferConnection
//go:generate go run go.uber.org/mock/mockgen -package application -destination updateseries_mocks_test.go github.com/juju/juju/apiserver/facades/client/application Application,Charm,UpdateBaseState,UpdateBaseValidator,CharmhubClient
//go:generate go run go.uber.org/mock/mockgen -package application -destination deployrepository_mocks_test.go github.com/juju/juju/apiserver/facades/client/application Bindings,DeployFromRepositoryState,DeployFromRepositoryValidator,Model,Machine
//go:generate go run go.uber.org/mock/mockgen -package application -destination repository_mocks_test.go github.com/juju/juju/core/charm Repository,RepositoryFactory
