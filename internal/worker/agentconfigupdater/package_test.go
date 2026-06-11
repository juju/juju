// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentconfigupdater_test

//go:generate go run github.com/canonical/gomock/mockgen -package agentconfigupdater_test -destination service_mock_test.go github.com/juju/juju/internal/worker/agentconfigupdater ControllerDomainServices,ControllerConfigService
