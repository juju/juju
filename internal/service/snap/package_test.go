// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package snap_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package snap -destination runnable_mock_test.go github.com/juju/juju/internal/service/snap Runnable
//go:generate go run go.uber.org/mock/mockgen -package snap -destination clock_mock_test.go github.com/juju/clock Clock

func Test(t *testing.T) {
	gc.TestingT(t)
}
