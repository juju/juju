// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -package caasapplicationprovisioner_test -destination service_mock_test.go github.com/juju/juju/apiserver/facades/controller/caasapplicationprovisioner ControllerConfigService,ModelConfigService,ModelInfoService,ApplicationService,StatusService
//go:generate go run go.uber.org/mock/mockgen -package caasapplicationprovisioner_test -destination leadership_mock_test.go github.com/juju/juju/core/leadership Revoker
//go:generate go run go.uber.org/mock/mockgen -package caasapplicationprovisioner_test -destination resource_opener_mock_test.go github.com/juju/juju/core/resource Opener

func TestAll(t *stdtesting.T) {
	tc.TestingT(t)
}
