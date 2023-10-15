// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package user_test -destination utils_controllercommand_mock_test.go github.com/juju/juju/cmd/juju/user ControllerCommand
//go:generate go run go.uber.org/mock/mockgen -package user_test -destination utils_clientstore_mock_test.go github.com/juju/juju/jujuclient ClientStore

// None of the tests in this package require mongo.
// Full command integration tests are found in cmd/juju/user_test.go

func Test(t *testing.T) {
	gc.TestingT(t)
}

var (
	GenerateUserControllerAccessToken = generateUserControllerAccessToken
)
