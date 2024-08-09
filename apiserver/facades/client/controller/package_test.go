// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	stdtesting "testing"

	"github.com/juju/juju/internal/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/state_mock.go github.com/juju/juju/apiserver/facades/client/controller Backend,Application,Relation
//go:generate go run go.uber.org/mock/mockgen -typed -package controller -destination service_mock_test.go github.com/juju/juju/apiserver/facades/client/controller ControllerAccessService,ControllerConfigService,ModelService,AgentService,ModelConfigService
//go:generate go run go.uber.org/mock/mockgen -typed -package controller -destination cloudspec_mock.go github.com/juju/juju/apiserver/common/cloudspec CloudSpecer

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
