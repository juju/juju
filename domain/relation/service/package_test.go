// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination leader_mock_test.go github.com/juju/juju/core/leadership Ensurer
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination package_mock_test.go github.com/juju/juju/domain/relation/service State,MigrationState,WatcherFactory
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination relation_mock_test.go github.com/juju/juju/domain/relation SubordinateCreator
