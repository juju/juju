// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package traceconfigupdater

//go:generate go run github.com/canonical/gomock/mockgen -package traceconfigupdater -destination tracing_api_mock_test.go github.com/juju/juju/internal/worker/traceconfigupdater TracingAPI
//go:generate go run github.com/canonical/gomock/mockgen -package traceconfigupdater -destination agent_mock_test.go github.com/juju/juju/agent Agent,Config
//go:generate go run github.com/canonical/gomock/mockgen -package traceconfigupdater -destination notify_watcher_mock_test.go github.com/juju/juju/core/watcher NotifyWatcher
