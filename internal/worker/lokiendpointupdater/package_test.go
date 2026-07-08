// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lokiendpointupdater

//go:generate go run github.com/canonical/gomock/mockgen -package lokiendpointupdater -destination logger_api_mock_test.go github.com/juju/juju/internal/worker/lokiendpointupdater LoggerAPI
//go:generate go run github.com/canonical/gomock/mockgen -package lokiendpointupdater -destination agent_mock_test.go github.com/juju/juju/agent Agent,Config
//go:generate go run github.com/canonical/gomock/mockgen -package lokiendpointupdater -destination notify_watcher_mock_test.go github.com/juju/juju/core/watcher NotifyWatcher
