// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

//go:generate go run github.com/canonical/gomock/mockgen -package service -destination package_mock_test.go github.com/juju/juju/domain/ssh/service State,WatcherFactory
//go:generate go run github.com/canonical/gomock/mockgen -package service -destination watcher_mock_test.go github.com/juju/juju/core/watcher StringsWatcher
