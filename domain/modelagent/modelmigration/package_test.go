// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

//go:generate go run github.com/canonical/gomock/mockgen -package modelmigration -destination package_mock_test.go github.com/juju/juju/domain/modelagent/modelmigration ImportService
