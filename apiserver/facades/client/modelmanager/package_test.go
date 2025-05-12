// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	stdtesting "testing"

	"github.com/juju/juju/internal/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package modelmanager_test -destination common_mock_test.go github.com/juju/juju/apiserver/common BlockCheckerInterface
//go:generate go run go.uber.org/mock/mockgen -typed -package modelmanager_test -destination domain_mock_test.go github.com/juju/juju/apiserver/common ControllerConfigService,BlockCommandService
//go:generate go run go.uber.org/mock/mockgen -typed -package modelmanager_test -destination migrator_mock_test.go github.com/juju/juju/apiserver/facades/client/modelmanager ModelExporter
//go:generate go run go.uber.org/mock/mockgen -typed -package modelmanager_test -destination service_mock_test.go github.com/juju/juju/apiserver/facades/client/modelmanager ApplicationService,AccessService,SecretBackendService,ModelService,DomainServicesGetter,ModelDefaultsService,ModelInfoService,ModelConfigService,NetworkService,ModelDomainServices,MachineService,ModelAgentService
//go:generate go run go.uber.org/mock/mockgen -typed -package modelmanager_test -destination status_mock_test.go github.com/juju/juju/apiserver/facades/client/modelmanager ModelStatusAPI

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
