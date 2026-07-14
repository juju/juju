// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater

//go:generate go run github.com/canonical/gomock/mockgen -package proxyupdater -destination controllermanifold_mock_test.go github.com/juju/juju/internal/worker/proxyupdater ControllerDomainServices,DomainServices,ModelService,ModelConfigService,ControllerNodeService
