// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

//go:generate go run github.com/canonical/gomock/mockgen -package service -destination controllerkey_mock_test.go github.com/juju/juju/domain/keyupdater/service ControllerKeyState
//go:generate go run github.com/canonical/gomock/mockgen -package service -destination service_mock_test.go github.com/juju/juju/domain/keyupdater/service ControllerKeyProvider,State,ControllerState
