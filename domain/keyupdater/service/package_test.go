// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination controllerkey_mock_test.go github.com/juju/juju/domain/keyupdater/service ControllerKeyState
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination service_mock_test.go github.com/juju/juju/domain/keyupdater/service ControllerKeyProvider,State,ControllerState

func TestPackage(t *stdtesting.T) {
	tc.TestingT(t)
}
