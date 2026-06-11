// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

//go:generate go run github.com/canonical/gomock/mockgen -package provider -destination package_mock_test.go github.com/juju/juju/domain/model/service/provider ControllerState,WatcherFactory
