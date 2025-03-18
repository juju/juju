// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package caasapplicationprovisioner_test -destination service_mock_test.go github.com/juju/juju/apiserver/facades/controller/caasapplicationprovisioner ControllerConfigService,ModelConfigService,ModelInfoService,ApplicationService,StatusService
//go:generate go run go.uber.org/mock/mockgen -package caasapplicationprovisioner_test -destination leadership_mock_test.go github.com/juju/juju/core/leadership Revoker
//go:generate go run go.uber.org/mock/mockgen -package caasapplicationprovisioner_test -destination resource_opener_mock_test.go github.com/juju/juju/core/resource Opener

func TestAll(t *testing.T) {
	gc.TestingT(t)
}
