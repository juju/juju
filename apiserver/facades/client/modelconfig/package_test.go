// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package modelconfig -destination service_mock.go github.com/juju/juju/apiserver/facades/client/modelconfig BlockCommandService,ModelAgentService,ModelConfigService,ModelSecretBackendService,ModelService
