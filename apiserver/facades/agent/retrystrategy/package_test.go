// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package retrystrategy_test

import (
	stdtesting "testing"

	coretesting "github.com/juju/juju/internal/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package retrystrategy_test -destination service_mock_test.go github.com/juju/juju/apiserver/facades/agent/retrystrategy ModelConfigService

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}
