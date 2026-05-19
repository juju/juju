// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration_test

//go:generate go run github.com/canonical/gomock/mockgen -package modelmigration -destination description_mock_test.go github.com/juju/description/v12 Model
//go:generate go run github.com/canonical/gomock/mockgen -package modelmigration -destination op_mock_test.go github.com/juju/juju/core/modelmigration Operation
