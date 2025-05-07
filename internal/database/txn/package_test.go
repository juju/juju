// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package txn

import (
	"testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package txn_test -destination clock_mock_test.go github.com/juju/clock Clock,Timer

func Test(t *testing.T) {
	tc.TestingT(t)
}
