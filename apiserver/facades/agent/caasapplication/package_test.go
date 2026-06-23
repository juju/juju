// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplication_test

//go:generate go run github.com/canonical/gomock/mockgen -package caasapplication -destination package_mock_test.go github.com/juju/juju/apiserver/facades/agent/caasapplication ControllerConfigService,ApplicationService,ModelAgentService,ControllerNodeService,TracingService,LokiConfigService
