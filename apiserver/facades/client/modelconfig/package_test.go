// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig

//go:generate go run github.com/canonical/gomock/mockgen -package modelconfig -destination service_mock.go github.com/juju/juju/apiserver/facades/client/modelconfig BlockCommandService,ModelAgentService,ModelConfigService,ModelSecretBackendService,ModelService
