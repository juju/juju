// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operationpruner

//go:generate go run go.uber.org/mock/mockgen -typed -package operationpruner -destination watcher_mock_test.go github.com/juju/juju/core/watcher StringsWatcher
//go:generate go run go.uber.org/mock/mockgen -typed -package operationpruner -destination services_mock_test.go github.com/juju/juju/internal/worker/operationpruner ModelConfigService,OperationService
