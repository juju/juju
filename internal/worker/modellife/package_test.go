// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modellife

import (
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/goleak"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package modellife -destination services_mock_test.go github.com/juju/juju/internal/worker/modellife ModelService

func Test(t *stdtesting.T) {
	defer goleak.VerifyNone(t)

	tc.TestingT(t)
}
