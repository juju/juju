// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

//go:generate go run go.uber.org/mock/mockgen -typed -package modelmigration -destination package_mock_test.go github.com/juju/juju/domain/crossmodelrelation/modelmigration ImportService
