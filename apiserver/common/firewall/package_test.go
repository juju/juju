// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewall_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package firewall_test -destination service_mock_test.go github.com/juju/juju/apiserver/common/firewall ModelConfigService

func TestAll(t *testing.T) {
	gc.TestingT(t)
}
