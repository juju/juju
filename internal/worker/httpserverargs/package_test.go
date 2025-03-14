// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserverargs_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package httpserverargs -destination services_mock_test.go github.com/juju/juju/internal/worker/httpserverargs ControllerConfigService,AccessService

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}
