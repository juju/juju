// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modellife

import (
	"testing"

	"go.uber.org/goleak"
	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package modellife -destination services_mock_test.go github.com/juju/juju/internal/worker/modellife ModelService

func Test(t *testing.T) {
	defer goleak.VerifyNone(t)

	gc.TestingT(t)
}
