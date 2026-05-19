// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operationpruner

//go:generate go run github.com/canonical/gomock/mockgen -package operationpruner -destination watcher_mock_test.go github.com/juju/juju/core/watcher StringsWatcher
//go:generate go run github.com/canonical/gomock/mockgen -package operationpruner -destination services_mock_test.go github.com/juju/juju/internal/worker/operationpruner ModelConfigService,OperationService
