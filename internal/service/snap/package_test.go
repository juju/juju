// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package snap_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package snap -destination runnable_mock_test.go github.com/juju/juju/internal/service/snap Runnable
//go:generate go run go.uber.org/mock/mockgen -typed -package snap -destination clock_mock_test.go github.com/juju/clock Clock

func Test(t *stdtesting.T) {
	tc.TestingT(t)
}
