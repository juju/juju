// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package logger_test -destination service_mock_test.go github.com/juju/juju/apiserver/facades/agent/logger ModelConfigService

func TestAll(t *stdtesting.T) {
	gc.TestingT(t)
}
