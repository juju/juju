// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestream

//go:generate go run go.uber.org/mock/mockgen -typed -package changestream -destination database_mock_test.go github.com/juju/juju/core/database TxnRunner
