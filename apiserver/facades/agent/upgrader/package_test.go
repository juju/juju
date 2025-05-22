// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader_test

import (
	"os"
	"testing"

	coretesting "github.com/juju/juju/internal/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package upgrader_test -destination domain_mock_test.go github.com/juju/juju/apiserver/facades/agent/upgrader ControllerConfigGetter,ModelAgentService,ControllerNodeService
//go:generate go run go.uber.org/mock/mockgen -typed -package upgrader -destination watch_mock.go github.com/juju/juju/apiserver/facades/agent/upgrader ModelAgentService
//go:generate go run go.uber.org/mock/mockgen -typed -package upgrader_test -destination upgrader_mock_test.go github.com/juju/juju/state Upgrader

func TestMain(m *testing.M) {
	os.Exit(func() int {
		defer coretesting.MgoTestMain()()
		return m.Run()
	}())
}
