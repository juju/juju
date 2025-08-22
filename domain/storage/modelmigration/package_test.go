// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

//go:generate go run go.uber.org/mock/mockgen -typed -package modelmigration -destination migrations_mock_test.go github.com/juju/juju/domain/storage/modelmigration Coordinator,ImportService,ExportService
//go:generate go run go.uber.org/mock/mockgen -typed -package modelmigration -destination storage_mock_test.go github.com/juju/juju/internal/storage ProviderRegistry
