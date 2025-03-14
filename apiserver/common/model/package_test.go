// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package model_test -destination service_mock_test.go github.com/juju/juju/apiserver/common/model MachineService,ModelConfigService

func TestAll(t *stdtesting.T) {
	gc.TestingT(t)
}
