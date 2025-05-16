// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package singular

import (
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/goleak"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package singular -destination lease_mock_test.go github.com/juju/juju/core/lease Manager,Claimer
//go:generate go run go.uber.org/mock/mockgen -typed -package singular -destination clock_mock_test.go github.com/juju/clock Clock
//go:generate go run go.uber.org/mock/mockgen -typed -package singular -destination agent_mock_test.go github.com/juju/juju/agent Agent,Config

func TestPackage(t *stdtesting.T) {
	defer goleak.VerifyNone(t)

	tc.TestingT(t)
}
