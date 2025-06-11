// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodeloperator

//go:generate go run go.uber.org/mock/mockgen -typed -package caasmodeloperator -destination service_mock_test.go github.com/juju/juju/apiserver/facades/controller/caasmodeloperator ControllerConfigService,ModelConfigService,AgentPasswordService,ControllerNodeService
