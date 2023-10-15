// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

func Test(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/lxdprofile_mock.go github.com/juju/charm/v8 LXDProfiler
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/state_mock.go github.com/juju/juju/state StorageAttachment,StorageInstance,MachinePortRanges,UnitPortRanges,CloudContainer
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/state_storage_mock.go github.com/juju/juju/state/storage Storage
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/storage_mock.go github.com/juju/juju/storage ProviderRegistry
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/poolmanager_mock.go github.com/juju/juju/storage/poolmanager PoolManager
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/leadership_mock.go github.com/juju/juju/core/leadership Reader
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/application_mock.go github.com/juju/juju/apiserver/facades/client/application Backend,StorageInterface,BlockChecker,Model,CaasBrokerInterface,Application,RemoteApplication,Charm,Relation,Unit,RelationUnit,Machine,Generation,Bindings
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/repository_mock.go github.com/juju/juju/apiserver/facades/client/application Repository
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/charm_mock.go github.com/juju/juju/apiserver/facades/client/application StateCharm
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/model_mock.go github.com/juju/juju/apiserver/facades/client/application StateModel
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/charmstore_mock.go github.com/juju/juju/apiserver/facades/client/application State
//go:generate go run go.uber.org/mock/mockgen -package application -destination updateseries_mocks_test.go github.com/juju/juju/apiserver/facades/client/application Application,Charm,UpdateSeriesState,UpdateSeriesValidator,CharmhubClient
