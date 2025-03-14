// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agenttools_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package agenttools -destination service_mock_test.go github.com/juju/juju/apiserver/facades/controller/agenttools ModelConfigService,ModelAgentService

func TestAll(t *stdtesting.T) {
	gc.TestingT(t)
}
