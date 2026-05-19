// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

//go:generate go run github.com/canonical/gomock/mockgen -package internal_test -destination watcher_mock_test.go github.com/juju/juju/apiserver/internal WatcherRegistry
