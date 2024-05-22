// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagecommon_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package imagecommon_test -destination service_mock_test.go github.com/juju/juju/apiserver/common/imagecommon ModelConfigService

func TestAll(t *stdtesting.T) {
	gc.TestingT(t)
}
