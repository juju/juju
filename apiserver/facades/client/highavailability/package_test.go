// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package highavailability

import (
	"os"
	stdtesting "testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package highavailability -destination service_mock_test.go github.com/juju/juju/apiserver/facades/client/highavailability ControllerNodeService,BlockCommandService
//go:generate go run go.uber.org/mock/mockgen -typed -package highavailability -destination auth_mock_test.go github.com/juju/juju/apiserver/facade Authorizer

func TestMain(m *stdtesting.M) {
	os.Exit(m.Run())
}
