// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration_test

//go:generate go run go.uber.org/mock/mockgen -typed -package modelmigration -destination description_mock_test.go github.com/juju/description/v12 Model
//go:generate go run go.uber.org/mock/mockgen -typed -package modelmigration -destination op_mock_test.go github.com/juju/juju/core/modelmigration Operation
