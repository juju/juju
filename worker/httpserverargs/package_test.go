// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserverargs_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package httpserverargs -destination controller_config_mock_test.go github.com/juju/juju/worker/httpserverargs ControllerConfigGetter

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
