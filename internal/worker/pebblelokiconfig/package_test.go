// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pebblelokiconfig

//go:generate go run github.com/canonical/gomock/mockgen -package pebblelokiconfig -destination pebble_client_mock_test.go github.com/juju/juju/internal/worker/pebblelokiconfig PebbleClient,LoggerAPI
//go:generate go run github.com/canonical/gomock/mockgen -package pebblelokiconfig -destination clock_mock_test.go github.com/juju/clock Clock
//go:generate go run github.com/canonical/gomock/mockgen -package pebblelokiconfig -destination agent_mock_test.go github.com/juju/juju/agent Agent,Config
//go:generate go run github.com/canonical/gomock/mockgen -package pebblelokiconfig -destination notify_watcher_mock_test.go github.com/juju/juju/core/watcher NotifyWatcher
