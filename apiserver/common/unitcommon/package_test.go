// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitcommon

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package unitcommon -destination service_mock_test.go github.com/juju/juju/apiserver/common/unitcommon ApplicationService

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
