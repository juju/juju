// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package deployer -destination domain_mock_test.go github.com/juju/juju/apiserver/facades/agent/deployer ControllerConfigGetter,ApplicationService,AgentPasswordService

func TestAll(t *testing.T) {
	gc.TestingT(t)
}
