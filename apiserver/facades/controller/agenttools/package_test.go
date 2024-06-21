// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agenttools_test

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package agenttools -destination service_mock_test.go github.com/juju/juju/apiserver/facades/controller/agenttools ModelConfigService,ModelAgentService

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
