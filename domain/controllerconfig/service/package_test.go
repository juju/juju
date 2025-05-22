// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination package_mock_test.go github.com/juju/juju/domain/controllerconfig/service State,WatcherFactory
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination watcher_mock_test.go github.com/juju/juju/core/watcher StringsWatcher
