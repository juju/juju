// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations

//go:generate go run go.uber.org/mock/mockgen -typed -package crossmodelrelations -destination package_mock_test.go github.com/juju/juju/apiserver/facades/controller/crossmodelrelations CrossModelRelationService,StatusService
//go:generate go run go.uber.org/mock/mockgen -typed -package crossmodelrelations -destination auth_mock_test.go github.com/juju/juju/apiserver/facade CrossModelAuthContext,MacaroonAuthenticator
//go:generate go run go.uber.org/mock/mockgen -typed -package crossmodelrelations -destination watcher_mock_test.go github.com/juju/juju/core/watcher NotifyWatcher
