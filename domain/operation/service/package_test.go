// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination service_mock_test.go github.com/juju/juju/domain/operation/service State,LeadershipService
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination objectstore_mock_test.go github.com/juju/juju/core/objectstore ModelObjectStoreGetter,ObjectStore
