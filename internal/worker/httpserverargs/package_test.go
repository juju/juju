// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserverargs_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package httpserverargs -destination controller_config_mock_test.go github.com/juju/juju/internal/worker/httpserverargs ControllerConfigGetter
//go:generate go run go.uber.org/mock/mockgen -package httpserverargs -destination user_service_mock_test.go github.com/juju/juju/internal/worker/httpserverargs UserService

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
