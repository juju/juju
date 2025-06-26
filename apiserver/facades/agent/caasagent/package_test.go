// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasagent_test

//go:generate go run go.uber.org/mock/mockgen -typed -package caasagent_test -destination services_mock_test.go github.com/juju/juju/apiserver/facades/agent/caasagent ModelService,ModelProviderService
//go:generate go run go.uber.org/mock/mockgen -typed -package caasagent_test -destination common_mock_test.go github.com/juju/juju/apiserver/common ControllerConfigService,ExternalControllerService,ControllerConfigState,APIHostPortsForAgentsGetter
//go:generate go run go.uber.org/mock/mockgen -typed -package caasagent_test -destination commonmodel_mock_test.go github.com/juju/juju/apiserver/common/model ModelConfigService
