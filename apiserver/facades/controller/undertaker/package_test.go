// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package undertaker -destination mock_service.go github.com/juju/juju/apiserver/facades/controller/undertaker SecretBackendService,ModelConfigService,ModelInfoService,ModelProviderService

