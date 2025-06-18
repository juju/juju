// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lifeflag_test

//go:generate go run go.uber.org/mock/mockgen -typed -package lifeflag_test -destination service_mock_test.go github.com/juju/juju/apiserver/facades/controller/lifeflag ApplicationService,MachineService
//go:generate go run go.uber.org/mock/mockgen -typed -package lifeflag_test -destination watcher_registry_mock_test.go github.com/juju/juju/apiserver/facade WatcherRegistry
