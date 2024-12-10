// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package txn

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package txn_test -destination clock_mock_test.go github.com/juju/clock Clock,Timer

func Test(t *testing.T) {
	gc.TestingT(t)
}
