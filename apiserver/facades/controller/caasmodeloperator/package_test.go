// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodeloperator

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package caasmodeloperator -destination service_mock_test.go github.com/juju/juju/apiserver/facades/controller/caasmodeloperator ControllerConfigService,ModelConfigService,AgentPasswordService

func TestAll(t *testing.T) {
	gc.TestingT(t)
}
