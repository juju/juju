// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package highavailability

import (
	stdtesting "testing"

	"github.com/juju/juju/internal/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package highavailability -destination common_mock_test.go github.com/juju/juju/apiserver/facades/client/highavailability ModelInfoService
func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
