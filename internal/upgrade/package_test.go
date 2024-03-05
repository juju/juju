// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrade

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package upgrade -destination lock_mock_test.go github.com/juju/juju/internal/worker/gate Lock
//go:generate go run go.uber.org/mock/mockgen -package upgrade -destination agent_mock_test.go github.com/juju/juju/agent Agent,Config

func TestAll(t *stdtesting.T) {
	gc.TestingT(t)
}
