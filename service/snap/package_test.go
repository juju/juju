// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package snap_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package snap -destination runnable_mock_test.go github.com/juju/juju/service/snap Runnable

func Test(t *testing.T) {
	gc.TestingT(t)
}
