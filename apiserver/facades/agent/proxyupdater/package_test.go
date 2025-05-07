// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater_test

import (
	"testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -package proxyupdater_test -destination service_mock_test.go github.com/juju/juju/apiserver/facades/agent/proxyupdater ControllerConfigService,ModelConfigService

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}
