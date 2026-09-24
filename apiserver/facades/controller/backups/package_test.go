// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

//go:generate go run github.com/canonical/gomock/mockgen -package backups -destination service_mock_test.go github.com/juju/juju/apiserver/facades/controller/backups ControllerExportService,ModelExportService,ModelExportDomainServices,ModelConfigService,ControllerModelLister,ControllerNodeLister
//go:generate go run github.com/canonical/gomock/mockgen -package backups -destination auth_mock_test.go github.com/juju/juju/apiserver/facade Authorizer
