// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestream

import (
	"testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package changestream -destination database_mock_test.go github.com/juju/juju/core/database TxnRunner

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}
