// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiaddresssetter

//go:generate go run go.uber.org/mock/mockgen -typed -package apiaddresssetter -destination package_mocks_test.go github.com/juju/juju/internal/worker/apiaddresssetter ControllerConfigService,ApplicationService,ControllerNodeService,NetworkService,ModelService,DomainServices,ControllerDomainServices
