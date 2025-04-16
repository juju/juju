// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasagent_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package caasagent_test -destination services_mock_test.go github.com/juju/juju/apiserver/facades/agent/caasagent StubService,ModelService,ModelConfigService,ControllerConfigService,ExternalControllerService,ControllerConfigState

func TestAll(t *testing.T) {
	gc.TestingT(t)
}
