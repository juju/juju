// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package retrystrategy_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package retrystrategy_test -destination service_mock_test.go github.com/juju/juju/apiserver/facades/agent/retrystrategy ModelConfigService

func TestAll(t *testing.T) {
	gc.TestingT(t)
}
