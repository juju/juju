// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewall_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package firewall_test -destination service_mock_test.go github.com/juju/juju/apiserver/common/firewall ModelConfigService

func TestAll(t *stdtesting.T) {
	tc.TestingT(t)
}
