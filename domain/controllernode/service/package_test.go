// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

//go:generate go run github.com/canonical/gomock/mockgen -package service -destination package_mock_test.go github.com/juju/juju/domain/controllernode/service State,WatcherFactory
