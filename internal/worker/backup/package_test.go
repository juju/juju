// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

//go:generate go run github.com/canonical/gomock/mockgen -package backup -destination service_mock_test.go github.com/juju/juju/internal/worker/backup ControllerExportService,ModelExportService,ModelExportDomainServices,MachineService,ModelConfigService,ControllerModelLister,ControllerNodeLister
