// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/common_mock.go github.com/juju/juju/apiserver/common BlockCheckerInterface
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/domain_mock.go github.com/juju/juju/apiserver/common ControllerConfigService,BlockCommandService
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/migrator_mock.go github.com/juju/juju/apiserver/facades/client/modelmanager ModelExporter
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/service_mock.go github.com/juju/juju/apiserver/facades/client/modelmanager ApplicationService,AccessService,SecretBackendService,ModelService,DomainServicesGetter,ModelDefaultsService,ModelInfoService,ModelConfigService,NetworkService,ModelDomainServices,MachineService,ModelAgentService

func TestAll(t *stdtesting.T) {
	gc.TestingT(t)
}
